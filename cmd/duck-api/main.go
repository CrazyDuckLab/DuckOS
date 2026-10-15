package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/api"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/healthauth"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/lifecycle"
	"github.com/CrazyDuckLab/DuckOS/internal/model"
	"github.com/CrazyDuckLab/DuckOS/internal/network"
	"github.com/CrazyDuckLab/DuckOS/internal/objectlisters"
	"github.com/CrazyDuckLab/DuckOS/internal/persist"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
	"github.com/CrazyDuckLab/DuckOS/internal/subscription"
	"github.com/CrazyDuckLab/DuckOS/internal/wireguard"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
	"github.com/CrazyDuckLab/DuckOS/pkg/brand"
	"github.com/CrazyDuckLab/DuckOS/pkg/buildinfo"
)

func main() {
	healthPublic := flag.String("updater-health-public-key", "", "Root-owned updater readiness verification key")
	listen := flag.String("listen", "127.0.0.1:8080", "Literal loopback management listener")
	socket := flag.String("agent-socket", "/run/duckos-agent/agent.sock", "Privileged Agent Unix socket")
	tokenFile := flag.String("token-file", "/etc/duckos/api-token", "Private access token file")
	agentAssertionKeyFile := flag.String("agent-assertion-key-file", "/etc/duckos/agent-assertion.key", "Private HMAC key for API-to-Agent authenticated principal assertions")
	identityDir := flag.String("identity-dir", "", "Optional pre-created private local identity directory")
	configDir := flag.String("config-dir", "/var/lib/duckos/config", "Canonical configuration journal directory")
	policyDir := flag.String("policy-dir", "/var/lib/duckos/policy", "Per-object policy overlay directory")
	wgCatalogDir := flag.String("wireguard-catalog-dir", "/var/lib/duckos/wireguard", "WireGuard durable catalog directory (read-only)")
	xrayCatalogDir := flag.String("xray-catalog-dir", "/var/lib/duckos/xray", "Xray durable catalog directory (read-only)")
	tlsCert := flag.String("tls-cert", "", "TLS certificate PEM with loopback IP SAN; requires --tls-key")
	tlsKey := flag.String("tls-key", "", "Private TLS key PEM; requires --tls-cert")
	webDir := flag.String("web-dir", "/opt/duckos/current/web", "Compiled dashboard directory")
	environment := flag.String("environment", "test", "dev, test or production")
	version := flag.Bool("version", false, "Print build metadata and exit")
	banner := flag.Bool("banner", false, "Print the DuckOS text mark and build metadata")
	flag.Parse()
	if *banner {
		fmt.Println(brand.Mark + "\n" + buildinfo.String())
		return
	}
	if *version {
		fmt.Println(buildinfo.String())
		return
	}
	if flag.NArg() != 0 || !model.ValidEnvironment(*environment) {
		log.Fatal("invalid arguments or environment")
	}
	if os.Geteuid() == 0 {
		log.Fatal("Duck API must run as an unprivileged user")
	}
	if _, err := api.ValidateListen(*listen); err != nil {
		log.Fatal(err)
	}
	token, err := api.LoadToken(*tokenFile)
	if err != nil {
		log.Fatal(err)
	}
	assertionKey, err := api.LoadToken(*agentAssertionKeyFile)
	if err != nil {
		log.Fatal(err)
	}
	eng := engine.New(*socket, *environment)
	// DV01 source-backed listers: open the wireguard / xray
	// durable catalogs (read-only access; the Agent owns
	// writes). A nil catalog produces honest engine_unavailable
	// on the corresponding /api/v1/objects/{kind} path; the
	// contract forbids fake-empty Complete=true on a missing
	// source.
	var wgCatalog *wireguard.FileCatalog
	if c, err := wireguard.OpenFileCatalog(*wgCatalogDir); err == nil {
		wgCatalog = c
	} else {
		log.Printf("wireguard catalog unavailable; listers will report engine_unavailable: %v", err)
	}
	var xcCatalog *xray.FileCatalog
	if c, err := xray.OpenFileCatalog(*xrayCatalogDir); err == nil {
		xcCatalog = c
	} else {
		log.Printf("xray catalog unavailable; listers will report engine_unavailable: %v", err)
	}
	objectlisters.RegisterAllWithRealSourcesAndLinuxSource(eng, wgCatalog, xcCatalog, nil, newAgentLinuxSource(*socket))
	// Wire the canonical config coordinator. The Agent
	// holds the privileged execution journal; the API
	// server mirrors it locally for revision/status reads.
	// The Config surface refuses any Prepare/Confirm/
	// Rollback until the verified Networking coordinator
	// is attached, so journal-only success stays closed.
	configStore, configErr := state.Open(*configDir, []byte(`{"version":1}`), func(raw json.RawMessage) error { _, err := network.Decode(raw); return err })
	if configErr == nil {
		defer configStore.Close()
		if cfg, err := eng.EnableConfig(configStore); err == nil {
			defer cfg.Close()
			if net, err := eng.EnableNetworking(configStore); err == nil {
				defer net.Close()
				_ = cfg.EnableNetworking(net)
			}
		}
	} else {
		log.Printf("config journal unavailable; Prepare/Confirm/Rollback will refuse: %v", configErr)
	}
	handler, err := api.New(eng, token, *listen, *webDir)
	// Public subscription delivery: /sub/<opaque-token>. The
	// token table is the SAME durable file the agent's center
	// issues into (shared state directory); content rendering is
	// delegated to the agent's center preview endpoint through
	// the existing authenticated IPC, so the public daemon never
	// touches the vault and delivery works with no operator
	// session.
	if h2 := handler; h2 != nil {
		if delivery := enableSubscriptionDelivery(*xrayCatalogDir, eng, assertionKey); delivery != nil {
			h2.SetSubscriptionDelivery(delivery)
		} else {
			log.Printf("subscription delivery disabled: token store unavailable under %s", *xrayCatalogDir)
		}
	}
	if err != nil {
		log.Fatal(err)
	}
	handler.EnableAgentAssertionKey(assertionKey)
	if *healthPublic != "" {
		key, err := healthauth.LoadPublic(*healthPublic)
		if err != nil {
			log.Fatal("updater readiness verification key unavailable")
		}
		handler.EnableUpdaterHealthKey(key)
	}
	defer handler.Close()
	var tlsConfig *tls.Config
	if *tlsCert != "" || *tlsKey != "" {
		if *tlsCert == "" || *tlsKey == "" {
			log.Fatal("both TLS certificate and private key are required")
		}
		tlsConfig, err = api.LoadTLSConfig(*tlsCert, *tlsKey)
		if err != nil {
			log.Fatal(err)
		}
		handler.EnableTLS()
	}
	if *identityDir != "" {
		identities, err := identity.Open(*identityDir)
		if err != nil {
			log.Fatal("Local identity store unavailable; inspect private state and permissions")
		}
		defer identities.Close()
		handler.EnableIdentity(identities)
	}
	// Production policy overlay: load the durable
	// per-object grant store. The store is the source of
	// truth for DV05 production_startup_loads_policy.
	policyStore, policyErr := openPolicyStoreForStartup(*policyDir)
	if policyErr != nil {
		log.Fatalf("policy overlay unavailable; refusing permissive authorization startup: %v", policyErr)
	} else {
		defer policyStore.Policy() // anchor
		handler.EnablePolicy(policyStore.Policy())
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	if tlsConfig != nil {
		listener = tls.NewListener(listener, tlsConfig)
	}
	log.Printf("%s; environment=%s; API listening on %s", buildinfo.String(), *environment, *listen)
	if err := lifecycle.Serve(listener, handler); err != nil {
		log.Fatal(err)
	}
}

func openPolicyStoreForStartup(policyDir string) (*identity.PolicyStore, error) {
	return identity.OpenPolicyStore(policyDir, identity.NewPolicy())
}

// enableSubscriptionDelivery builds the public /sub handler. The
// token store is opened read-write from the shared Xray state
// directory; content is rendered by the agent through the IPC
// proxy. A nil return means delivery is not available and must be
// reported honestly, not faked.
func enableSubscriptionDelivery(catalogDir string, eng *engine.Engine, assertionKey string) http.Handler {
	tokens := subscription.NewTokenStore()
	tokenFile, err := persist.Open[[]subscription.TokenRecord](filepath.Join(catalogDir, "center"), "tokens.json")
	if err != nil {
		return nil
	}
	tokens.SetPersist(func(recs []subscription.TokenRecord) error {
		return tokenFile.Update(func(cur *[]subscription.TokenRecord) { *cur = recs })
	})
	tokens.LoadRecords(tokenFile.Get())
	delivery := subscription.NewDeliveryHandler(tokens, nil)
	delivery.Prefix = "/sub"
	// The delivery daemon acts as the dedicated service principal
	// "duck-api-delivery" with network-admin rights, scoped to the
	// single render endpoint; it never forwards subscriber tokens
	// into the management plane.
	serviceToken := identity.ForwardedToken(assertionKey, identity.User{
		Username: "duck-api-delivery",
		Role:     identity.NetworkAdmin,
	})
	delivery.Render = func(subID string) ([]byte, error) {
		// Subscription records issued by the management surface may
		// reference either a group or a profile-based subscription;
		// try the group shape first, then the profile shape.
		if data, err := renderThroughIPC(eng, "group", subID, subscription.FormatBase64, serviceToken); err == nil {
			return data, nil
		}
		return renderThroughIPC(eng, "subscription", subID, subscription.FormatBase64, serviceToken)
	}
	delivery.RenderFormat = func(subID string, format subscription.Format) ([]byte, error) {
		if data, err := renderThroughIPC(eng, "group", subID, format, serviceToken); err == nil {
			return data, nil
		}
		return renderThroughIPC(eng, "subscription", subID, format, serviceToken)
	}
	return delivery
}

func renderThroughIPC(eng *engine.Engine, kind, subID string, format subscription.Format, token string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path := "/api/v1/xray/center/preview/" + kind + "/" + url.PathEscape(subID)
	rawQuery := ""
	if format != "" && format != subscription.FormatBase64 {
		rawQuery = "format=" + url.QueryEscape(string(format))
	}
	status, data, _, err := eng.XrayProxy(ctx, http.MethodGet, path, rawQuery, token, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("render returned status %d", status)
	}
	return data, nil
}
