package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/CrazyDuckLab/DuckOS/internal/healthauth"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SystemdRuntime has no caller-controlled commands, units, URLs or executable
// paths. It manages only the installed DuckOS Agent and API units.
type SystemdRuntime struct {
	Root, HealthKeyFile string
	HealthTimeout       time.Duration
}

func systemctl(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "/usr/bin/systemctl", args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C"}
	cmd.WaitDelay = time.Second
	var out boundedOutput
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", errors.New("upgrade: systemd operation failed")
	}
	return strings.TrimSpace(out.String()), nil
}
func (r *SystemdRuntime) Restart(ctx context.Context) error {
	_, err := systemctl(ctx, "restart", "duck-agent.service", "duck-api.service")
	return err
}
func (r *SystemdRuntime) Stop(ctx context.Context) error {
	_, err := systemctl(ctx, "stop", "duck-api.service", "duck-agent.service")
	return err
}

func (r *SystemdRuntime) Health(ctx context.Context, build BuildIdentity) error {
	timeout := r.HealthTimeout
	if timeout <= 0 || timeout > 90*time.Second {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := r.probe(ctx, build); err == nil && ctx.Err() == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("upgrade: runtime health/build verification failed")
		case <-ticker.C:
		}
	}
}

func fileDigest(path string) ([32]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxPayloadBytes {
		return [32]byte{}, errors.New("upgrade: invalid executable")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, err
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}

func (r *SystemdRuntime) probe(ctx context.Context, build BuildIdentity) error {
	for _, name := range []string{"duck-agent", "duck-api"} {
		pidText, err := systemctl(ctx, "show", "--property=MainPID", "--value", name+".service")
		if err != nil {
			return err
		}
		pid, err := strconv.Atoi(pidText)
		if err != nil || pid < 2 {
			return errors.New("upgrade: managed process absent")
		}
		live, err := fileDigest(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
		if err != nil {
			return err
		}
		wanted, err := fileDigest(filepath.Join(r.Root, "current", "bin", name))
		if err != nil || wanted != live {
			return errors.New("upgrade: running executable is not selected release")
		}
	}
	key, err := healthauth.LoadPrivate(r.HealthKeyFile)
	if err != nil {
		return errors.New("upgrade: readiness credential unavailable")
	}
	defer clear(key)
	request, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:8080/api/v1/ready", nil)
	if err != nil {
		return err
	}
	assertion := healthauth.Sign(key, time.Now())
	if assertion == "" {
		return errors.New("upgrade: readiness assertion unavailable")
	}
	request.Header.Set("Authorization", "Bearer "+assertion)
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("upgrade: readiness probe failed")
	}
	defer response.Body.Close()
	var payload struct {
		Status  string `json:"status"`
		Version string `json:"buildVersion"`
		Commit  string `json:"buildCommit"`
	}
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&payload) != nil || payload.Status != "ready" || payload.Version != build.Version || payload.Commit != build.Commit {
		return errors.New("upgrade: running API build/readiness mismatch")
	}
	return nil
}
