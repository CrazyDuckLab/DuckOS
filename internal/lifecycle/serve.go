package lifecycle

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func Serve(listener net.Listener, handler http.Handler) error {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 12 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if err := server.Shutdown(shutdown); err != nil {
		_ = server.Close()
		return err
	}
	return nil
}
