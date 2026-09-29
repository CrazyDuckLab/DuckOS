//go:build linux

package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/restore"
)

func main() {
	if os.Geteuid() != 0 || len(os.Args) != 1 {
		log.Fatal("restore requires the root service; no runtime arguments accepted")
	}
	// Deployment-owned constants match the unit's exact write allowlist. The
	// canonical Engine journal is deliberately outside the managed file root.
	service, err := restore.Open("/var/lib/duckos/restore-source", "/var/lib/duckos/restore-live", "/var/lib/duckos-restore")
	if err != nil {
		for _, code := range []syscall.Errno{syscall.EACCES, syscall.EPERM, syscall.EROFS, syscall.EXDEV, syscall.ELOOP, syscall.ENOENT, syscall.EINVAL, syscall.ENOSYS} {
			if errors.Is(err, code) {
				log.Printf("restore startup filesystem error: %s", code)
				break
			}
		}
		log.Fatal("restore startup/recovery failed; private journal preserved")
	}
	defer service.Close()
	listener, err := agent.Listen("/run/duckos-restore/restore.sock", 0)
	if err != nil {
		log.Fatal("restore local IPC unavailable")
	}
	defer listener.Close()
	if err := os.Chmod("/run/duckos-restore/restore.sock", 0600); err != nil {
		log.Fatal("restore socket permissions failed")
	}
	server := &http.Server{Handler: service, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 70 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 8192}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 65*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal("restore listener stopped")
	}
}
