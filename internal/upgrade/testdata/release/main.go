package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/api"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/healthauth"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/observe"
	"github.com/CrazyDuckLab/DuckOS/pkg/buildinfo"
	"net"
	"net/http"
	"os"
	"strings"
)

var version = "v1"
var commit = "1111111111111111111111111111111111111111"
var unhealthy = "false"

func main() {
	production := flag.Bool("production-handler", false, "")
	agentSocket := flag.String("agent-socket", "", "")
	identityDir := flag.String("identity-dir", "", "")
	healthPublic := flag.String("health-public-key", "", "")
	web := flag.String("web-dir", "", "")
	show := flag.Bool("version", false, "")
	socket := flag.String("socket", "", "")
	address := flag.String("listen", "", "")
	tokenFile := flag.String("token-file", "", "")
	flag.Parse()
	if *show {
		fmt.Printf("DuckOS %s (%s)\n", version, commit)
		return
	}
	network, target := "unix", *socket
	if *address != "" {
		network, target = "tcp", *address
	}
	if network == "unix" {
		_ = os.Remove(target)
	}
	listener, err := net.Listen(network, target)
	if err != nil {
		panic(err)
	}
	if *production {
		_ = http.Serve(listener, agent.NewHandler(&observe.Collector{Environment: "test"}))
		return
	}
	if *identityDir != "" {
		buildinfo.Version, buildinfo.Commit = version, commit
		token, err := api.LoadToken(*tokenFile)
		if err != nil {
			panic(err)
		}
		h, err := api.New(engine.New(*agentSocket, "test"), token, *address, *web)
		if err != nil {
			panic(err)
		}
		defer h.Close()
		auth, err := identity.Open(*identityDir)
		if err != nil {
			panic(err)
		}
		defer auth.Close()
		h.EnableIdentity(auth)
		key, err := healthauth.LoadPublic(*healthPublic)
		if err != nil {
			panic(err)
		}
		h.EnableUpdaterHealthKey(key)
		_ = http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if unhealthy == "true" {
				w.WriteHeader(503)
				return
			}
			h.ServeHTTP(w, r)
		}))
		return
	}
	_ = http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if *tokenFile != "" {
			token, err := os.ReadFile(*tokenFile)
			if err != nil || r.Header.Get("Authorization") != "Bearer "+strings.TrimSpace(string(token)) {
				w.WriteHeader(401)
				return
			}
		}
		if unhealthy == "true" {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready", "version": version, "commit": commit, "buildVersion": version, "buildCommit": commit})
	}))
}
