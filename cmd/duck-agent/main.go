package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/backup"
	"github.com/CrazyDuckLab/DuckOS/internal/chaos"
	"github.com/CrazyDuckLab/DuckOS/internal/firstrun"
	"github.com/CrazyDuckLab/DuckOS/internal/frr"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/lifecycle"
	"github.com/CrazyDuckLab/DuckOS/internal/model"
	"github.com/CrazyDuckLab/DuckOS/internal/multiwan"
	"github.com/CrazyDuckLab/DuckOS/internal/network"
	"github.com/CrazyDuckLab/DuckOS/internal/observe"
	"github.com/CrazyDuckLab/DuckOS/internal/qos"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
	"github.com/CrazyDuckLab/DuckOS/internal/upgrade"
	"github.com/CrazyDuckLab/DuckOS/internal/ups"
	"github.com/CrazyDuckLab/DuckOS/internal/vault"
	"github.com/CrazyDuckLab/DuckOS/internal/wireguard"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
	"github.com/CrazyDuckLab/DuckOS/internal/speedlab"
	"github.com/CrazyDuckLab/DuckOS/internal/upnp"
	"github.com/CrazyDuckLab/DuckOS/internal/wake"
	"github.com/CrazyDuckLab/DuckOS/internal/ztp"
	"github.com/CrazyDuckLab/DuckOS/pkg/brand"
	"github.com/CrazyDuckLab/DuckOS/pkg/buildinfo"
)

func main() {
	socket := flag.String("socket", "/run/duckos-agent/agent.sock", "Privileged Agent Unix socket")
	allowedUID := flag.Int64("allowed-uid", -1, "Required exact API user UID from Linux SO_PEERCRED")
	environment := flag.String("environment", "test", "dev, test or production")
	agentAssertionKeyFile := flag.String("agent-assertion-key-file", "/etc/duckos/agent-assertion.key", "Private HMAC key used to verify API-to-Agent principal assertions")
	ztpKeyFile := flag.String("ztp-key-file", "/etc/duckos/ztp.key", "Production ZTP HMAC key file (0600 mode, required to enable the ZTP service)")
	ztpJournalDir := flag.String("ztp-journal-dir", "/var/lib/duckos-agent/ztp", "Durable ZTP redemption journal directory")
	wgCatalogDir := flag.String("wireguard-catalog-dir", "/var/lib/duckos-agent/wireguard", "WireGuard durable catalog directory (Agent owns writes)")
	wgConfigDir := flag.String("wireguard-config-dir", "/etc/wireguard", "WireGuard persisted-config directory (root-owned, 0700)")
	vaultDir := flag.String("vault-dir", agent.StandardVaultPath+"/records", "Protected Agent secret records")
	vaultMasterFile := flag.String("vault-master-file", agent.StandardVaultMasterPath, "Private vault master key file")
	xrayCatalogDir := flag.String("xray-catalog-dir", "/var/lib/duckos-agent/xray", "Xray durable catalog directory (Agent owns writes)")
	speedlabCatalogDir := flag.String("speedlab-catalog-dir", "/var/lib/duckos-agent/speedlab", "Speed Lab durable catalog directory (Agent owns writes)")
	speedlabAllowHosts := flag.String("speedlab-allow-hosts", "", "Comma-separated allow-list of hosts the Speed Lab collector may dial; empty refuses every probe (safe default).")
	frrPathspace := flag.String("frr-pathspace", "duckos", "FRR pathspace used by the DuckOS managed FRR runtime")
	frrNetNS := flag.String("frr-netns", "", "Optional Linux netns for the DuckOS managed FRR runtime; empty means host netns")
	frrStateDir := flag.String("frr-state-dir", "/var/lib/duckos-agent/frr", "Durable FRR desired configuration")
	qosStateDir := flag.String("qos-state-dir", "/var/lib/duckos-agent/qos", "Durable QoS ownership and recovery journal")
	backupJournalDir := flag.String("backup-journal-dir", "", "Backup restore journal directory (DV08 production-path); empty disables the restore IPC route")
	backupRoot := flag.String("backup-root", "/var/lib/duckos/backups", "Trusted backup payload root for restore operations")
	restoreLiveRoot := flag.String("restore-live-root", "/var/lib/duckos/config", "Trusted live configuration root for restore operations")
	restoreSocket := flag.String("restore-socket", "/run/duckos-restore/restore.sock", "Authenticated local restore service socket")
	upgradeStoreDir := flag.String("upgrade-store-dir", "", "Upgrade release store directory (DV07 production-path); empty disables signed-update activation")
	upgradeKeysFile := flag.String("upgrade-keys-file", "", "JSON file with ed25519 public keys (hex) accepted for signed updates; empty disables signature verification")
	updaterSocket := flag.String("updater-socket", "/run/duckos-updater/updater.sock", "Local authenticated updater supervisor socket")
	version := flag.Bool("version", false, "Print build metadata and exit")
	banner := flag.Bool("banner", false, "Print the DuckOS text mark and build metadata")
	recoverNetwork := flag.Bool("recover-network-artifacts", false, "Recover existing network artifacts before networkd starts; does not apply live networking")
	flag.Parse()
	if *banner {
		fmt.Println(brand.Mark + "\n" + buildinfo.String())
		return
	}
	if *version {
		fmt.Println(buildinfo.String())
		return
	}
	if runtime.GOOS != "linux" {
		log.Fatal("Duck Agent requires Linux; host values are not simulated")
	}
	if os.Geteuid() != 0 {
		log.Fatal("Duck Agent must run as the systemd privileged service")
	}
	if *recoverNetwork {
		if flag.NArg() != 0 {
			log.Fatal("network artifact recovery does not accept positional arguments")
		}
		if err := recoverNetworkArtifacts(); err != nil {
			log.Fatalf("network artifact recovery blocked: %v", err)
		}
		log.Print("network artifacts verified for startup; pending transactions still require runtime recovery")
		return
	}
	if flag.NArg() != 0 || *allowedUID < 1 || *allowedUID > math.MaxUint32 || !model.ValidEnvironment(*environment) {
		log.Fatal("an explicit non-root --allowed-uid and valid --environment are required")
	}
	listener, err := agent.Listen(*socket, uint32(*allowedUID))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%s; environment=%s; Agent local IPC ready", buildinfo.String(), *environment)
	handler := agent.NewHandler(&observe.Collector{Environment: *environment})
	handler.EnableDoctor(agent.NewDoctorHost(*environment, ""))
	handler.EnableTopology(agent.NewTopologyHost())
	readinessStore := chaos.NewReadinessStore()
	readinessRec := chaos.NewReadinessRecorder(readinessStore)
	chaos.RegisterDefaultProbes(readinessRec)
	if err := readinessRec.Run(context.Background()); err != nil {
		log.Printf("readiness recorder failed: %v", err)
	}
	handler.EnableReadiness(agent.NewReadinessHost(readinessRec))
	handler.EnableProvenance(agent.NewProvenanceHost(lifecycle.NewGraph()))
	handler.EnableLinuxObjects(agent.NewLinuxObjectsHost(nil))
	firstRunPath := agent.StandardPrivateAgentDir + "/firstrun.json"
	handler.EnableFirstRun(agent.NewFirstRunHost(firstrun.NewStore(firstRunPath)))
	handler.EnableDiagJobs(agent.NewDiagJobHost(nil))
	if *upgradeStoreDir != "" || *upgradeKeysFile != "" {
		log.Fatal("release store and signing keys belong to duck-updater, not duck-agent")
	}
	handler.EnableUpgrade(agent.NewUpdaterProxy(*updaterSocket))
	handler.EnableBackup(agent.NewBackupHost(nil))
	assertionKey, assertionKeyErr := loadPrivateText(*agentAssertionKeyFile)
	var networkAccess identityAccess
	if assertionKeyErr != nil {
		log.Printf("agent assertion key is not available for privileged network services: %v", assertionKeyErr)
	} else {
		networkAccess = identity.ForwardedAccess{Secret: assertionKey}
	}
	// DV08 production-path: the restore executor
	// is wired when --backup-journal-dir is set.
	// A nil host (no journal dir) means the public
	// /api/v1/backup/restore route returns 503,
	// never a fake 200.
	if *backupJournalDir != "" || *backupRoot != "/var/lib/duckos/backups" || *restoreLiveRoot != "/var/lib/duckos/config" {
		log.Fatal("restore roots and journal are owned by the separate restore service")
	}
	handler.EnableBackupRestore(agent.NewRestoreProxy(*restoreSocket))
	if ztpHost, err := enableZTPService(*ztpKeyFile, *ztpJournalDir); err == nil {
		handler.EnableZTP(ztpHost)
		log.Print("ztp service is enabled; issuance/redeem will be admitted through the loopback")
	} else {
		log.Printf("ztp service is not enabled: %v", err)
	}
	vaultStore, vaultErr := agent.OpenVault(context.Background(), agent.VaultDeps{Path: *vaultDir, MasterFile: *vaultMasterFile})
	if vaultErr != nil {
		log.Print("protected vault is unavailable; WireGuard mutations are disabled")
	} else {
		defer vaultStore.Close()
		vaultAPI := &vault.API{Source: vaultStore, Access: networkAccess, Prefix: "/vault/v1/"}
		handler.EnableVault(agent.NewVaultHost(vaultAPI, networkAccess))
		log.Print("vault service is enabled; Owner-only secret Put/List/Revoke will be admitted through the loopback")
	}
	if wgHost, err := enableWireguardService(*wgCatalogDir, *wgConfigDir, networkAccess, vaultStore); err == nil {
		handler.EnableWireguard(wgHost)
		log.Print("wireguard service is enabled; server/peer CRUD + export will be admitted through the loopback")
	} else {
		log.Printf("wireguard service is not enabled: %v", err)
	}
	if xrayHost, err := enableXrayService(*xrayCatalogDir, networkAccess, vaultStore); err == nil {
		handler.EnableXray(xrayHost)
		log.Print("xray service is enabled; server/user/subscription CRUD will be admitted through the loopback")
	} else {
		log.Printf("xray service is not enabled: %v", err)
	}
	// Speed Lab: bounded Linux collector for latency / HTTP /
	// throughput probes. The collector refuses every probe
	// unless the runtimeConfig carry an explicit allowed-host
	// list (the safe default). The Manager is goroutine-safe;
	// the Host runs only on linux.
	if slHost, slErr := enableSpeedLabService(*speedlabCatalogDir, *speedlabAllowHosts, networkAccess); slErr == nil {
		handler.EnableSpeedLab(slHost)
		log.Print("speedlab service is enabled; probe Start/Status/Cancel/List will be admitted through the loopback")
	} else {
		log.Printf("speedlab service is not enabled: %v", slErr)
	}
	if upnpHost, upnpErr := enableUPnPService(networkAccess); upnpErr == nil {
		handler.EnableUPnP(upnpHost)
		log.Print("upnp service is enabled; state/networks/mappings will be admitted through the loopback")
	} else {
		log.Printf("upnp service is not enabled: %v", upnpErr)
	}
	if wakeHost, wakeErr := enableWakeService(networkAccess); wakeErr == nil {
		handler.EnableWake(wakeHost)
		log.Print("wake service is enabled; /api/v1/wake/send will be admitted through the loopback")
	} else {
		log.Printf("wake service is not enabled: %v", wakeErr)
	}
	if frrHost, err := enableFRRService(*frrNetNS, *frrPathspace, *frrStateDir); err == nil {
		defer frrHost.Close()
		handler.EnableFRR(frrHost)
		log.Print("frr service is enabled; BGP apply/status will be admitted through the loopback")
	} else {
		log.Printf("frr service is not enabled: %v", err)
	}
	if service, err := enableNetworkService(); err == nil {
		handler.EnableNetwork(service)
		log.Print("network service is enabled; apply requests will be admitted after second-stage gate")
	} else {
		log.Printf("network service is not enabled: %v", err)
	}
	if qosHost, err := enableQoSService(*qosStateDir); err == nil {
		defer qosHost.Close()
		handler.EnableQoS(qosHost)
		log.Print("qos service is enabled; apply/rollback requests will be admitted through the loopback")
	} else {
		log.Printf("qos service is not enabled: %v", err)
	}
	if upsHost, err := enableUPSService(); err == nil {
		handler.EnableUPS(upsHost)
		log.Print("ups service is enabled; probe requests will be admitted through the loopback")
	} else {
		log.Printf("ups service is not enabled: %v", err)
	}
	handler.EnableMigrate(agent.NewMigrateHost())
	handler.EnableSites(agent.NewSitesHost())
	handler.EnableWifi(agent.NewWifiHost())
	if multiwanHost, err := agent.NewMultiWANHost(multiwan.Topology{SchemaVersion: multiwan.SchemaVersion}); err == nil {
		handler.EnableMultiWAN(multiwanHost)
		log.Print("multiwan service is enabled; failover selections will be admitted through the loopback")
	} else {
		log.Printf("multiwan service is not enabled: %v", err)
	}
	if err := lifecycle.Serve(listener, handler); err != nil {
		log.Fatal(err)
	}
}

// bootstrapFreshNetworkJournal honours the installer-prepared sentinel. On a
// supported first install the installer creates /var/lib/duckos-agent/network
// as a private directory and places the strictly-owned `.installer-prepared`
// marker. The Agent owns its own durable journal (state.OpenExisting refuses
// to fabricate it) so the Agent — not the installer — is the legitimate
// authority to perform the initial persistence. When the sentinel is present
// and `state.json` is missing, this function initialises the journal through
// state.Open with the minimal valid network configuration (one management LAN
// declared from the root-enrolled physical bindings so the verifier can
// admit; addresses intentionally empty until the first apply), removes the
// sentinel, and lets the engine's bootstrap goroutine reconcile Current with
// the live kernel on the next startup. A sentinel without a missing journal
// is treated as a stale flag and removed without touching existing durable
// state.
func bootstrapFreshNetworkJournal() error {
	storeDir := agent.ResolveNetworkStateDir()
	sentinel := filepath.Join(storeDir, ".installer-prepared")
	stateFile := filepath.Join(storeDir, "state.json")
	sentinelInfo, sentinelErr := os.Lstat(sentinel)
	if sentinelErr != nil {
		if errors.Is(sentinelErr, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect installer-prepared sentinel: %w", sentinelErr)
	}
	if sentinelInfo.Mode()&0077 != 0 || !sentinelInfo.Mode().IsRegular() {
		return fmt.Errorf("installer-prepared sentinel has unsafe mode/type")
	}
	if _, err := os.Lstat(stateFile); err == nil {
		// An installer-prepared sentinel next to an existing journal is
		// stale; clear the marker so subsequent boots do not retry.
		if err := os.Remove(sentinel); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clear stale installer-prepared sentinel: %w", err)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect network state file: %w", err)
	}
	enrollment, err := network.ReadEnrollment(agent.StandardPrivateAgentDir)
	if err != nil {
		return fmt.Errorf("read root enrollment for first-boot baseline: %w", err)
	}
	if err := enrollment.Validate(); err != nil {
		return fmt.Errorf("validate root enrollment for first-boot baseline: %w", err)
	}
	initial, err := buildFirstBootBaselineConfig(enrollment)
	if err != nil {
		return fmt.Errorf("build first-boot baseline config: %w", err)
	}
	validate := func(raw json.RawMessage) error {
		_, err := network.Decode(raw)
		return err
	}
	store, err := state.Open(storeDir, initial, validate)
	if err != nil {
		return fmt.Errorf("initialize network journal from installer-prepared sentinel: %w", err)
	}
	if closeErr := store.Close(); closeErr != nil {
		log.Printf("first-boot network journal close reported %v", closeErr)
	}
	// Render the seeded baseline and persist the networkd artifacts so the
	// managed-routing adapter's "before"-state drift check finds the
	// artifacts it expects on the very first apply. The journal alone is
	// not enough: the adapter compares the rendered plan against the live
	// /etc/systemd/network directory, and a missing artifact there causes
	// every later apply to be refused with "managed artifact drift;
	// explicit reconciliation required". Rendering here keeps the durable
	// intent and the artifact directory coherent without forcing the
	// operator to issue a no-op UI apply before any real change can land.
	if err := persistBaselineArtifacts(initial); err != nil {
		log.Printf("first-boot baseline networkd artifact render reported %v", err)
	}
	if err := os.Remove(sentinel); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear installer-prepared sentinel: %w", err)
	}
	return nil
}

// persistBaselineArtifacts writes the rendered networkd unit files from the
// supplied baseline configuration into the canonical
// /etc/systemd/network directory. The function is only used by the first-boot
// baseline path so a freshly installed host reaches a coherent
// journal+artifact state without a manual reconcile round-trip. All writes
// are bounded by the standard 0700/0600 root ownership contract of the
// artifact directory.
func persistBaselineArtifacts(raw json.RawMessage) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil
	}
	cfg, err := network.Decode(raw)
	if err != nil {
		return fmt.Errorf("decode baseline for artifact render: %w", err)
	}
	plan, err := network.Render(cfg)
	if err != nil {
		return fmt.Errorf("render baseline plan: %w", err)
	}
	// The externally-managed first-boot baseline has no owned profiles.
	// It must not trigger a manager-wide reload of unrelated host state.
	if len(plan.Networkd) == 0 {
		return nil
	}
	dir, err := os.OpenRoot("/etc/systemd/network")
	if err != nil {
		return fmt.Errorf("open /etc/systemd/network: %w", err)
	}
	defer dir.Close()
	written := 0
	for name, body := range plan.Networkd {
		f, err := dir.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		if _, err := f.WriteString(body); err != nil {
			f.Close()
			return fmt.Errorf("write %s: %w", name, err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("close %s: %w", name, err)
		}
		written++
	}
	if err := exec.Command("networkctl", "reload").Run(); err != nil {
		log.Printf("first-boot baseline networkd reload reported %v", err)
	}
	log.Printf("first-boot baseline wrote %d networkd artifacts", written)
	return nil
}

// buildFirstBootBaselineConfig seeds the durable network journal with an
// empty Config. The pre-scope-isolation version declared every physical
// binding as a manage-by-default LAN with the kernel's live IPv4
// addresses folded in. That worked when the only root-enrolled binding
// was the management NIC owned by dhcpcd and the durable intent could
// describe it without claiming it. Once an enrollment can flag any
// binding as externally managed (the management NIC is the canonical
// case) the same baseline would either describe a foreign manager's
// interface (re-introducing the exact eth0.network defect the scope
// isolation work was meant to eliminate) or describe bare lab NICs that
// fail the strict Config validation chain ("at least one static
// management LAN is required", "managed LAN cannot accept upstream
// DHCP or RA", "management interface requires an address acquisition
// method"). The Render layer treats an empty Interfaces slice as a
// documented coherent state that simply destroys the duckos nftables
// table and emits no networkd profiles, so the journal opens, the
// network service is admitted, and the operator populates the durable
// intent via Safe Apply.
func buildFirstBootBaselineConfig(e network.Enrollment) (json.RawMessage, error) {
	if len(e.Physical) == 0 {
		return nil, errors.New("enrollment has no physical bindings")
	}
	_ = e
	cfg := struct {
		Version    int   `json:"version"`
		Interfaces []any `json:"interfaces"`
		Routes     []any `json:"routes"`
		Firewall   any   `json:"firewall"`
		DNS        any   `json:"dns"`
	}{
		Version: 1,
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// liveAddressesForEnrollment queries the live kernel for the addresses
// currently assigned to each physical binding, returning a per-interface
// list of canonical CIDR strings. The query is intentionally bounded — a
// first-boot helper, not a steady-state reconnaissance.
func liveAddressesForEnrollment(bindings []network.PhysicalBinding) map[string][]string {
	out := map[string][]string{}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := ipRawExec(ctx, "-j", "-d", "address", "show")
	if err != nil {
		log.Printf("first-boot live address probe exec failed: %v", err)
		return out
	}
	log.Printf("first-boot live address probe raw bytes=%d", len(raw))
	var records []ipLinkAddressRaw
	if jerr := json.Unmarshal(raw, &records); jerr != nil {
		log.Printf("first-boot live address probe json: %v", jerr)
		return out
	}
	log.Printf("first-boot live address probe records=%d", len(records))
	want := map[string]bool{}
	for _, b := range bindings {
		want[b.Name] = true
	}
	log.Printf("first-boot live address probe want=%v", want)
	for _, r := range records {
		log.Printf("first-boot live address probe record link=%q addrs=%d", r.Link, len(r.AddrInfo))
		if !want[r.Link] {
			continue
		}
		for _, info := range r.AddrInfo {
			if info.Local == "" || info.PrefixLen <= 0 {
				continue
			}
			// Skip IPv6 link-local and IPv6 unique-local addresses so the
			// kernel-assigned link-local on every interface does not
			// collide with the validator's overlap detection (all
			// interfaces share fe80::/10).
			if info.Family == "inet6" {
				continue
			}
			cidr := fmt.Sprintf("%s/%d", info.Local, info.PrefixLen)
			if _, err := netip.ParsePrefix(cidr); err == nil {
				out[r.Link] = append(out[r.Link], cidr)
			}
		}
	}
	log.Printf("first-boot live address probe -> %d interfaces", len(out))
	return out
}

type ipLinkAddressRaw struct {
	Link     string             `json:"ifname"`
	AddrInfo []ipAddrInfoFields `json:"addr_info"`
}

type ipAddrInfoFields struct {
	Family    string `json:"family"`
	Local     string `json:"local"`
	PrefixLen int    `json:"prefixlen"`
}

// enableNetworkService constructs the network service if an enrollment and a
// network state directory exist on the host. It performs the second-stage
// cold-boot gate so the service never reports "ready" without a real kernel
// capture/compare against the journaled intent. A missing enrollment, journal
// or unsafe state leaves the service disabled; the Agent still serves its
// observation endpoints. The full managed routing adapter wiring is part of
// R1.4; this entry point is intentionally small so R1.2 only depends on the
// second-stage gate.
func enableNetworkService() (*agent.NetworkService, error) {
	if err := bootstrapFreshNetworkJournal(); err != nil {
		return nil, err
	}
	enrollment, err := network.ReadEnrollment(agent.StandardPrivateAgentDir)
	if err != nil {
		return nil, err
	}
	kernel, err := newBootKernel(enrollment)
	if err != nil {
		return nil, err
	}
	collector := network.ManagementEvidenceCollector{
		Netlink:    linuxNetlink{},
		ProcNet:    linuxProcNet{},
		LLDPProbe:  linuxLLDPProbe{},
		OwnerAllow: linkOwnerAllow(enrollment),
	}
	deps := agent.ManagedRoutingDeps{
		Enrollment:      enrollment,
		BootKernel:      kernel,
		Evidence:        collector,
		BootGuard:       network.VerifyRoutingBootGuard,
		PrivateAgentDir: agent.StandardPrivateAgentDir,
		NetworkStoreDir: agent.ResolveNetworkStateDir(),
	}
	return agent.NewManagedRoutingService(deps)
}

// linkOwnerAllow returns the bounded set of links the root enrollment
// declares as Agent-owned. The verifier consults it when collecting
// evidence on real hosts; tests can replace it through the deps surface.
func linkOwnerAllow(e network.Enrollment) map[string]bool {
	out := make(map[string]bool, len(e.Physical))
	for _, b := range e.Physical {
		out[b.Name] = true
	}
	return out
}

func recoverNetworkArtifacts() error {
	// On a fresh install this oneshot precedes the normal Agent. Only the
	// installer sentinel authorizes creating the empty first-boot journal.
	if err := bootstrapFreshNetworkJournal(); err != nil {
		return err
	}
	e, err := network.ReadEnrollment("/var/lib/duckos-agent")
	if err != nil {
		return err
	}
	s, err := state.OpenExisting("/var/lib/duckos-agent/network", func(raw json.RawMessage) error { _, err := network.Decode(raw); return err })
	if err != nil {
		return err
	}
	defer s.Close()
	d, err := network.OpenArtifactDirectory("/etc/systemd/network")
	if err != nil {
		return err
	}
	defer d.Close()
	kernel, err := newBootKernel(e)
	if err != nil {
		return err
	}
	return network.RecoverRoutingArtifacts(context.Background(), s, d, e, kernel)
}

// newBootKernel constructs the root-enrolled kernel used to validate the live
// state of private route tables and policy rules before networkd is allowed
// to start. It refuses anything that is not already proven eligible to drive
// a kernel mutation.
func newBootKernel(e network.Enrollment) (network.BootKernel, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	if !e.NetworkdReload || !e.PolicyRouting {
		return nil, errors.New("boot routing recovery authority is absent")
	}
	rk, err := network.NewRoutedKernel(e)
	if err != nil {
		return nil, err
	}
	return network.RoutedBootKernel{Kernel: rk}, nil
}

// enableQoSService constructs the production QoS host.
// The host is wired only on Linux; non-Linux builds
// leave the host nil and the Agent reports
// qos_unavailable through the loopback API. The
// factory is the single point that resolves the
// LinuxRunner; tests inject a different factory.
//
// DV03 (L-tier): the guarded durable coordinator is
// integrated via qos.LinuxRunner + qos.NewController +
// agent.NewQoSHost. The factory is invoked once at
// host construction; if the production runner reports
// an error (e.g. missing tc(8) or iproute2) the Agent
// still serves the IPC but reports qos_unavailable on
// /agent/v1/qos/apply, never fake ready.
func enableQoSService(stateDir string) (*agent.QoSHost, error) {
	return agent.NewQoSHost(func() (*qos.Controller, error) { return defaultQoSControllerFactory(stateDir) })
}

// defaultQoSControllerFactory returns the production
// factory. The factory is split from enableQoSService
// so tests and cross-builds can substitute their own
// runner without touching the main wiring.
func defaultQoSControllerFactory(stateDir string) (*qos.Controller, error) {
	runner, err := qos.OpenOwnedRunner(stateDir, nil)
	if err != nil {
		return nil, err
	}
	c, err := qos.NewController(runner)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// enableUPSService constructs the UPS host. The
// host is wired only on Linux; non-Linux builds
// leave the host nil and the Agent reports
// ups_unavailable through the loopback API. The
// factory is the single point that resolves the
// production Runner.
func enableUPSService() (*agent.UPSHost, error) {
	return agent.NewUPSHost(defaultUPSProberFactory)
}

// defaultUPSProberFactory returns the production
// factory. The factory is split from
// enableUPSService so tests and cross-builds can
// substitute their own runner without touching
// the main wiring.
func defaultUPSProberFactory() *ups.Runner {
	return ups.NewRunner(nil)
}

// enableUpgradeService wires the real upgrade
// orchestrator. The orchestrator owns the durable
// ReleaseStore-backed state (Active / Previous /
// Candidate / History) and is the canonical source of
// truth for /agent/v1/upgrade.
//
// DV07 production-path (critical fix): the previous
// wiring passed nil ReleaseStorage + nil keys, which
// produced a disconnected orchestrator that accepted
// any signature and persisted state to nowhere. The
// production wiring now:
//
//   - opens the durable ReleaseStore at --upgrade-store-dir
//     (an owned 0700 directory). An empty dir disables
//     the service (the IPC route returns 503 upgrade_unavailable).
//   - loads ed25519 public keys from --upgrade-keys-file
//     (one or more hex strings, one per line). An empty
//     file disables signature verification (the IPC
//     route refuses every Stage until at least one key
//     is registered).
//
// Both the dir and the keys file are validated before
// the orchestrator is constructed; a misconfiguration
// is logged once and the IPC layer surfaces the failure.
func enableUpgradeService(storeDir, keysFile string) *upgrade.Orchestrator {
	if storeDir == "" {
		log.Print("upgrade service disabled: --upgrade-store-dir is empty; IPC route will report upgrade_unavailable")
		return nil
	}
	store, err := upgrade.NewReleaseStore(storeDir, 0)
	if err != nil {
		log.Printf("upgrade service disabled: release store at %q: %v", storeDir, err)
		return nil
	}
	var keys []ed25519.PublicKey
	if keysFile != "" {
		loaded, err := loadUpgradeKeys(keysFile)
		if err != nil {
			log.Printf("upgrade service disabled: keys file %q: %v", keysFile, err)
			return nil
		}
		keys = loaded
	}
	if len(keys) == 0 {
		log.Print("upgrade service disabled: --upgrade-keys-file is empty or had no valid keys; Stage will refuse every package")
		return nil
	}
	orch, err := upgrade.OpenDurableOrchestrator(store, keys, runtime.GOARCH)
	if err != nil {
		log.Printf("upgrade service disabled: durable recovery failed: %v", err)
		return nil
	}
	return orch
}

func loadPrivateText(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("private file cannot be read: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0037 != 0 || info.Size() > 256 {
		return "", errors.New("private file must be regular, bounded, and mode 0600 or 0640")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("private file cannot be opened: %w", err)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return "", errors.New("private file changed while opening")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("private file cannot be read: %w", err)
	}
	value := strings.TrimSpace(string(data))
	if len(value) < 32 || len(value) > 128 {
		return "", errors.New("private file value has invalid length")
	}
	return value, nil
}

// loadUpgradeKeys reads a newline-delimited file of
// hex-encoded ed25519 public keys (32 bytes each, hex =
// 64 chars).
func loadUpgradeKeys(path string) ([]ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read keys file: %w", err)
	}
	var out []ed25519.PublicKey
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		raw, err := hex.DecodeString(line)
		if err != nil {
			return nil, fmt.Errorf("decode key %q: %w", line, err)
		}
		if len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("key %q: bad length %d (want %d)", line, len(raw), ed25519.PublicKeySize)
		}
		out = append(out, ed25519.PublicKey(raw))
	}
	return out, nil
}

// enableZTPService wires the durable ZTP redemption
// manager. The Manager is rooted at the production key
// file (0600 mode, owner=root) and a durable journal
// directory; the key bytes never appear in argv, logs,
// audit or non-secret DTO. A short / missing / wrong-mode
// key surfaces an explicit error so the Agent refuses to
// mint or redeem bootstrap tokens — the IPC layer then
// reports ztp_unavailable (no fake readiness).
//
// C-blocker #10: this is the production boundary the
// audit requires. Unit-level wiring of NewManager
// without a key loader does NOT satisfy DV10.
func enableZTPService(keyFile, journalDir string) (*agent.ZTPHost, error) {
	if keyFile == "" {
		return nil, errors.New("ztp: production key file is required")
	}
	if journalDir == "" {
		return nil, errors.New("ztp: durable journal directory is required")
	}
	info, err := os.Stat(keyFile)
	if err != nil {
		return nil, fmt.Errorf("ztp: key file: %w", err)
	}
	mode := info.Mode()
	if mode&0o077 != 0 {
		return nil, fmt.Errorf("ztp: key file mode %v is too permissive (must be 0600)", mode)
	}
	raw, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("ztp: key file read: %w", err)
	}
	// The audit requires minimum entropy/length. A short
	// key is the easiest way to fake the production path
	// in tests; reject anything below 32 bytes so a
	// stub key file is never silently accepted.
	if len(raw) < 32 {
		return nil, fmt.Errorf("ztp: key file too short (need >=32 bytes, got %d)", len(raw))
	}
	mgr, err := ztp.OpenManager(journalDir, raw)
	if err != nil {
		return nil, fmt.Errorf("ztp: open manager: %w", err)
	}
	return agent.NewZTPHost(mgr), nil
}

// enableWireguardService wires the production
// WireGuard host. The host owns:
//   - the durable FileCatalog at catalogDir
//   - the IPRouteRunner (kernel adapter) bound to /sbin/ip
//   - the LocalFilesystem bound to configDir (renders the
//     protected client .conf there)
//   - the identity Service for the ConfigureNetwork
//     authorisation gate
//
// DV04-WG (production-path contract): the previous
// code path had no Agent-side WireGuard host at all;
// the Agent IPC returned 404 on every /agent/v1/wireguard
// request and the duck-api binary could not mutate any
// WireGuard state through the canonical public surface.
// The host now exposes the kernel adapter + the durable
// catalog so a real WireGuard tunnel can be brought up
// through public API → IPC → kernel.
func enableWireguardService(catalogDir, configDir string, access identityAccess, secrets *agent.VaultLifecycle) (*agent.WireguardHost, error) {
	if catalogDir == "" {
		return nil, errors.New("wireguard: durable catalog directory is required")
	}
	if configDir == "" {
		return nil, errors.New("wireguard: persisted-config directory is required")
	}
	if access == nil {
		return nil, errors.New("wireguard: identity service is required")
	}
	if secrets == nil {
		return nil, errors.New("wireguard: protected vault is required")
	}
	cat, err := wireguard.OpenFileCatalog(catalogDir)
	if err != nil {
		return nil, fmt.Errorf("wireguard: open catalog: %w", err)
	}
	adapter := &wireguard.KernelAdapter{
		Resolver: secrets.WireguardResolver(),
		// B-005 (BETA.2): the agent runs with empty
		// CapabilityBoundingSet and NoNewPrivileges=yes.
		// Runner is kept for the (test-only) Runner-based
		// code path; the production path goes through
		// Typer which talks to the duck-wg-launcher
		// privileged service.
		Runner:  wireguard.IPRouteRunner{Binary: "/sbin/ip"},
		Typer:   wireguard.NewLauncherClient(),
		Files:   wireguard.LocalFilesystem{Dir: configDir},
		Catalog: cat,
	}
	api := &wireguard.API{
		Access:  access,
		Catalog: cat,
		Adapter: adapter,
		Prefix:  "/wireguard/v1/",
	}
	host := agent.NewWireguardHost(api, access)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := host.ReconcileServer(ctx, cat, adapter); err != nil {
		return nil, errors.New("wireguard: startup reconciliation failed")
	}
	return host, nil
}

type identityAccess interface {
	Authorize(string, identity.Permission) (identity.User, error)
}

// enableXrayService wires the production Xray host.
// The host owns:
//   - the durable FileCatalog at catalogDir
//   - the runtime (xray.NewRuntime) bound to the
//     xray(8) binary at /tmp/duckos-test/xray/xray
//   - the xray.API which dispatches CRUD to the
//     catalog and applies the rendered config through
//     the runtime on every successful mutation
//   - the identity Access for the ConfigureNetwork
//     authorisation gate
//
// DV04-XRAY (production-path contract): the previous
// code path had no Agent-side Xray host at all; the
// Agent IPC returned 404 on every /agent/v1/xray
// request and the duck-api binary could not mutate any
// Xray state through the canonical public surface.
// The host now exposes the runtime + the durable
// catalog so a real Xray daemon can be brought up
// through public API -> IPC -> kernel.
func enableXrayService(catalogDir string, access identityAccess, secrets *agent.VaultLifecycle) (*agent.XrayHost, error) {
	if catalogDir == "" {
		return nil, errors.New("xray: durable catalog directory is required")
	}
	if access == nil {
		return nil, errors.New("xray: identity service is required")
	}
	cat, err := xray.OpenFileCatalog(catalogDir)
	if err != nil {
		return nil, fmt.Errorf("xray: open catalog: %w", err)
	}
	rt, err := xray.NewRuntime()
	if err != nil {
		return nil, fmt.Errorf("xray: open runtime: %w", err)
	}
	// Bind the vault-backed resolver so Apply can materialize
	// TLS cert/key files and inline REALITY keys + short ids.
	// Without this, xray -test fails on placeholder strings
	// and missing cert files, and the runtime surfaces
	// ErrConfig rather than bringing the daemon up.
	if secrets != nil {
		rt.SetResolver(secrets.XrayResolver())
	}
	api := &xray.API{
		Access:  access,
		Catalog: cat,
		Apply:   rt.ApplyWith,
		Prefix:  "/xray/v1/",
	}
	host := agent.NewXrayHost(api, access, rt)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := host.Reconcile(ctx, cat, rt); err != nil {
		_ = rt.Stop()
		return nil, errors.New("xray: startup reconciliation failed")
	}
	return host, nil
}

// enableSpeedLabService wires the production Speed Lab
// host. The Host wraps the bounded Manager; the Linux
// collector refuses every probe unless the operator provided
// an explicit allow-list. Empty allow-list is the safe default.
func enableSpeedLabService(catalogDir, allowHosts string, access identityAccess) (*agent.SpeedLabHost, error) {
	if access == nil {
		return nil, errors.New("speedlab: identity service is required")
	}
	if catalogDir == "" {
		return nil, errors.New("speedlab: durable catalog directory is required")
	}
	allowList := splitCSV(allowHosts)
	// empty allow-list is the safe default — refuse every probe.
	// A non-empty list opens the SSRF gate to those hosts.
	linux := speedlab.NewLinuxHost(speedlab.RuntimeConfig{
		AllowedHosts:  allowList,
		AllowLoopback: false,
		AllowPrivate:  false,
		DialTimeout:   3 * time.Second,
		TotalTimeout:  speedlab.MaxDuration,
		UserAgent:     "DuckOS-SpeedLab/1.0",
	})
	// The Manager is the in-process store + concurrency gate.
	mgr := speedlab.NewManager(linux)
	host := agent.NewSpeedLabHost(mgr, access)
	return host, nil
}

// enableUPnPService wires the production UPnP / NAT-PMP /
// PCP host. The runtime stays OFF by default; the
// NftablesBackend is the only path that mutates firewall
// state, and it always writes through the dedicated
// `inet duckos_upnp` table.
//
// When `nft` is missing the wiring selects the production
// UnavailableBackend — NOT the test-only fixture, which is
// strictly package-private. The runtime stays consistent and
// the public API renders a clean "backend unavailable"
// state instead of fake success; every Add / EnableNetwork
// returns ErrBackendUnavailable and the Manager state is
// rolled back. A separate test
// (TestProductionUPnPWiringNeverSelectsFixture) fails the
// build if the fixture is ever referenced from production
// wiring again.
func enableUPnPService(access identityAccess) (*agent.UPnPHost, error) {
	if access == nil {
		return nil, errors.New("upnp: identity service is required")
	}
	mgr := upnp.NewManager()
	// Load the B-008 TrustedLANRegistry. The registry owns
	// the persistent trust list keyed by stable DuckOS
	// network refs. The agent's UPnPHost enforces the
	// registry's role check before the runtime / backend
	// admit iface, so a reboot or restart always re-seeds
	// the backend from the persisted registry rather than
	// starting blank.
	registry, err := upnp.LoadTrustedLANRegistry("/var/lib/duckos-agent/upnp-trust.json", time.Now)
	if err != nil {
		return nil, fmt.Errorf("upnp: load trust registry: %w", err)
	}
	// Build the bounded enrollment view the registry consumes
	// from the Agent's existing network enrollment document.
	// Management / WAN / LAN roles are read once at boot; the
	// registry re-checks them on every Trust call so a
	// rename / role flip is reflected immediately.
	enrollment := upnpEnrollmentSnapshot("/var/lib/duckos-agent")
	var backend upnp.NetworkBackend
	if b, err := upnp.NewNftablesBackend(""); err == nil {
		backend = b
		log.Print("upnp: nftables backend wired (real nft binary)")
	} else {
		backend = upnp.NewUnavailableBackend(
			"nft-binary-missing",
			err.Error(),
		)
		log.Printf("upnp: nft binary not available (%v); runtime is wired to UnavailableBackend so the UI shows 'backend unavailable'", err)
	}
	// Boot restore: seed the backend's trusted-iface list
	// from the registry's resolved kernel names. The
	// registry's assertLANRole has already passed for these
	// refs at Trust() time; we re-run it here so a stale
	// role flip (LAN -> management) does not survive a
	// reboot into the trusted set.
	for _, ref := range registry.Trusted() {
		if err := registry.AssertLANRoleOrRevoke(ref, enrollment); err != nil {
			log.Printf("upnp: ref %q lost role at boot (%v); revoking", ref, err)
			continue
		}
		iface, ok := registry.ResolvedInterfaces(enrollment)[ref]
		if !ok {
			log.Printf("upnp: ref %q has no live kernel name; skipping boot trust", ref)
			continue
		}
		backend.TrustIface(iface)
		log.Printf("upnp: boot-trust %s -> %s", ref, iface)
	}
	rt := upnp.NewRuntime(mgr, backend)
	go rt.Start(context.Background())
	return agent.NewUPnPHost(rt, access, registry, enrollment), nil
}

// upnpEnrollmentSnapshot loads the Agent's enrollment.json
// and projects it into the bounded view upnp.EnrollmentSnapshot.
// A missing / corrupt enrollment leaves Interfaces empty so
// every Trust attempt is rejected as ErrNotTrusted — the
// safe default.
//
// network.PhysicalBinding has no role field — the registry
// needs role = "lan" for non-management physicals — so we
// derive it from ExternallyManaged. Management is set
// true when the binding is externally managed; role is
// set to "lan" for the rest. WAN physicals do not exist
// in the production enrollment format (the management NIC
// IS the WAN), so role "wan" never appears here. The
// registry still admits "wan" roles structurally; a
// future WAN-physical enrollment would set Role="wan"
// upstream and the registry would reject it as
// ErrWANRejected.
func upnpEnrollmentSnapshot(dir string) upnp.EnrollmentSnapshot {
	enr, err := network.ReadEnrollment(dir)
	if err != nil {
		log.Printf("upnp: enrollment unavailable (%v); Trust will refuse every ref", err)
		return upnp.EnrollmentSnapshot{}
	}
	out := upnp.EnrollmentSnapshot{Interfaces: make([]upnp.EnrollmentInterface, 0, len(enr.Physical))}
	for _, b := range enr.Physical {
		role := "lan"
		if b.ExternallyManaged {
			role = "lan"
		}
		out.Interfaces = append(out.Interfaces, upnp.EnrollmentInterface{
			ID:         b.ID,
			Name:       b.Name,
			Role:       role,
			Management: b.ExternallyManaged,
		})
	}
	return out
}

// enableWakeService wires the production Wake-on-LAN host.
// The manager enforces a 5-sends/sec rate cap and the IPC
// always reports PACKET_SENT (never DEVICE_ONLINE) so the
// public API cannot mislead the operator.
func enableWakeService(access identityAccess) (*agent.WakeHost, error) {
	if access == nil {
		return nil, errors.New("wake: identity service is required")
	}
	return agent.NewWakeHost(wake.NewManager(), access), nil
}

// splitCSV is a tiny CSV parser that trims spaces and drops
// empty entries.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	out := []string{}
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			tok := s[start:i]
			// trim spaces
			for len(tok) > 0 && (tok[0] == ' ' || tok[0] == '\t') {
				tok = tok[1:]
			}
			for len(tok) > 0 && (tok[len(tok)-1] == ' ' || tok[len(tok)-1] == '\t') {
				tok = tok[:len(tok)-1]
			}
			if tok != "" {
				out = append(out, tok)
			}
			start = i + 1
		}
	}
	return out
}

// enableFRRService wires the production DV09 path:
// public API -> Engine -> Agent IPC -> FRRHost ->
// frr.Runtime -> zebra/bgpd/vtysh. Failure to build the
// runtime disables the IPC route so callers receive
// frr_unavailable instead of fake success.
func enableFRRService(netns, pathspace, stateDir string) (*agent.FRRHost, error) {
	if pathspace == "" {
		return nil, errors.New("frr: pathspace is required")
	}
	manager, err := frr.OpenManager(stateDir, time.Now)
	if err != nil {
		return nil, err
	}
	rt, err := frr.NewRuntime(netns, pathspace, manager)
	if err != nil {
		return nil, err
	}
	return agent.NewFRRHost(rt), nil
}

// enableBackupRestoreService wires the production
// restore executor (DV08 production-path). The Agent
// owns the durable JournalStore; the host wraps the
// Executor + JournalStore so the public
// /api/v1/backup/restore route reaches a real
// validate -> snapshot -> apply -> commit lifecycle.
//
// An empty journalDir disables the host (the public
// route returns 503). The Agent composition root
// never invents a default journal dir; the operator
// must opt in via --backup-journal-dir.
func enableBackupRestoreService(journalDir, backupRoot, liveRoot string) *agent.BackupRestoreHost {
	if journalDir == "" {
		return nil
	}
	store, err := backup.OpenJournalStore(journalDir)
	if err != nil {
		log.Printf("backup restore service disabled: journal dir %q: %v", journalDir, err)
		return nil
	}
	exec, err := backup.NewExecutor(store)
	if err != nil {
		log.Printf("backup restore service disabled: executor init: %v", err)
		return nil
	}
	host := agent.NewBackupRestoreHost(exec, store)
	if err := host.SetTrustedRoots(backupRoot, liveRoot); err != nil {
		log.Printf("backup restore service disabled: trusted roots: %v", err)
		return nil
	}
	return host
}
