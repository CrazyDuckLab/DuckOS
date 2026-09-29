package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/healthauth"
	"github.com/CrazyDuckLab/DuckOS/internal/upgrade"
)

func main() {
	initHealth := flag.Bool("init-health-key", false, "Provision root-owned health-only signing key and API public key once")
	root := flag.String("release-root", "/opt/duckos", "Trusted installed release root")
	storeDir := flag.String("store-dir", "/var/lib/duckos-updater", "Private verified package/workflow store")
	keyFile := flag.String("keys-file", "/etc/duckos/update-keys", "Root-owned Ed25519 public trust keys, one hex key per line")
	socket := flag.String("socket", "/run/duckos-updater/updater.sock", "Local root-only IPC")
	enroll := flag.Bool("enroll-installed", false, "Verify the installed baseline against private store/bootstrap.duck; does not replace files")
	flag.Parse()
	if os.Geteuid() != 0 || runtime.GOOS != "linux" || flag.NArg() != 0 {
		log.Fatal("updater requires Linux root service and named options")
	}
	if *initHealth {
		if err := healthauth.Provision(healthauth.PrivatePath, healthauth.PublicPath); err != nil {
			log.Fatal("health key provisioning failed; existing files preserved")
		}
		return
	}
	info, err := os.Lstat(*keyFile)
	if err != nil {
		log.Fatal("updater trust keys unavailable")
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() > 65536 {
		log.Fatal("unsafe updater trust file")
	}
	raw, err := os.ReadFile(*keyFile)
	if err != nil {
		log.Fatal("updater trust keys unreadable")
	}
	var keys []ed25519.PublicKey
	for _, line := range strings.Fields(string(raw)) {
		b, err := hex.DecodeString(line)
		if err != nil || len(b) != ed25519.PublicKeySize {
			log.Fatal("invalid updater trust key")
		}
		keys = append(keys, ed25519.PublicKey(b))
	}
	if len(keys) == 0 {
		log.Fatal("updater requires trusted signing keys")
	}
	store, err := upgrade.NewReleaseStore(*storeDir, 64<<20)
	if err != nil {
		log.Fatal("updater store unavailable")
	}
	if *enroll {
		bootstrap := filepath.Join(*storeDir, "bootstrap.duck")
		info, err := os.Lstat(bootstrap)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<20 {
			log.Fatal("invalid baseline package")
		}
		raw, err := os.ReadFile(bootstrap)
		if err != nil {
			log.Fatal("baseline package unreadable")
		}
		if err := upgrade.EnrollInstalledRelease(store, keys, runtime.GOARCH, *root, &upgrade.SystemdRuntime{Root: *root, HealthKeyFile: healthauth.PrivatePath}, raw); err != nil {
			log.Fatal("signed baseline enrollment failed; installed release was not replaced")
		}
		return
	}
	supervisor, err := upgrade.OpenSupervisor(store, keys, runtime.GOARCH, *root, &upgrade.SystemdRuntime{Root: *root, HealthKeyFile: healthauth.PrivatePath})
	if err != nil {
		log.Fatal("updater startup recovery failed; private journal preserved")
	}
	defer supervisor.Close()
	listener, err := agent.Listen(*socket, 0)
	if err != nil {
		log.Fatal("updater IPC unavailable")
	}
	defer listener.Close()
	if err := os.Chmod(*socket, 0600); err != nil {
		log.Fatal("updater IPC permissions failed")
	}
	server := &http.Server{Handler: &upgrade.SupervisorHTTP{Supervisor: supervisor}, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8192}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal("updater IPC stopped unexpectedly")
	}
}
