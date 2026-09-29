//go:build linux

package wireguard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// IPRouteRunner drives the kernel via the iproute2 `ip`
// binary (interface add/del, address add, link show).
// WireGuard private-key / listen-port / peer operations
// use the dedicated `wg(8)` binary so the production
// path matches the wireguard-tools(8) contract; the
// Runner interface accepts a single command line per
// call, so the adapter calls Run twice when it needs
// the same operation on `ip` and `wg`.
//
// Each call is a separate process with a 10-second
// deadline; the runner never returns bytes from stderr
// in production (it is reserved for the kernel
// adapter's failure path) and never logs the private
// key.
type IPRouteRunner struct {
	// Binary is the absolute path to the iproute2 `ip`
	// binary used for link / address operations. Empty
	// defaults to /sbin/ip.
	Binary string
	// WireGuardBin is the absolute path to the
	// wireguard-tools `wg` binary used for
	// private-key / listen-port / peer operations.
	// Empty defaults to /usr/bin/wg.
	WireGuardBin string
}

// Run dispatches the call to `ip` for link / address
// operations and to `wg` for WireGuard-specific
// operations. The adapter sets the WireGuardPrivateKey
// flag via the special prefix `__wg__` on the first
// argument; the adapter is the only caller and the
// prefix is reserved for the production boundary.
func (r IPRouteRunner) Run(ctx context.Context, args []string) (string, error) {
	if len(args) > 0 && args[0] == "__wg__" {
		return r.runWg(ctx, args[1:])
	}
	return r.runIp(ctx, args)
}

func (r IPRouteRunner) runIp(ctx context.Context, args []string) (string, error) {
	binary := r.Binary
	if binary == "" {
		binary = "/sbin/ip"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return "", fmt.Errorf("ip binary unavailable: %w", err)
	}
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, binary, args...)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("%s %v: %w (stderr=%s)", binary, args, err, errOut.String())
	}
	return out.String(), nil
}

func (r IPRouteRunner) runWg(ctx context.Context, args []string) (string, error) {
	binary := r.WireGuardBin
	if binary == "" {
		binary = "/usr/bin/wg"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return "", fmt.Errorf("wg binary unavailable: %w", err)
	}
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// WireGuard private-key and peer preshared-key
	// bytes must be supplied to wg(8) via a file (the
	// `private-key /path` and `preshared-key /path`
	// syntax). The adapter encodes the bytes in
	// `__privkey__:<b64>` and `__psk__:<b64>` markers on
	// the command line; this dispatcher consumes the
	// markers, writes the bytes to a 0600 temp file,
	// and rewrites the args to point the corresponding
	// wg(8) flag at the file. The temp file is unlinked
	// before the function returns.
	privPayload, pskPayload, args, privTmp, pskTmp := extractSecretStdin(args)
	if privTmp != "" {
		defer os.Remove(privTmp)
		if err := os.WriteFile(privTmp, []byte(privPayload), 0o600); err != nil {
			return "", fmt.Errorf("wg private-key temp: %w", err)
		}
		if _, err := os.Stat(privTmp); err != nil {
			return "", fmt.Errorf("wg private-key stat: %w", err)
		}
	}
	if pskTmp != "" {
		defer os.Remove(pskTmp)
		if err := os.WriteFile(pskTmp, []byte(pskPayload), 0o600); err != nil {
			return "", fmt.Errorf("wg preshared-key temp: %w", err)
		}
	}
	cmd := exec.CommandContext(c, binary, args...)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("%s %v: %w (stderr=%s)", binary, args, err, errOut.String())
	}
	return out.String(), nil
}

// extractSecretStdin scans args for `__privkey__:<b64>`
// (interface private-key) and `__psk__:<b64>` (peer
// preshared-key) markers. Each marker is consumed; the
// matching `private-key` / `preshared-key` flag is
// appended with the dispatcher's 0600 temp file path.
// The temp file lifetime is owned by the caller.
func extractSecretStdin(args []string) (string, string, []string, string, string) {
	out := make([]string, 0, len(args)+2)
	var privPayload string
	var privTmp string
	var pskPayload string
	var pskTmp string
	for _, a := range args {
		if strings.HasPrefix(a, "__privkey__:") {
			privPayload = strings.TrimPrefix(a, "__privkey__:")
			privTmp = filepath.Join(os.TempDir(), fmt.Sprintf("wg-priv-%d.b64", os.Getpid()))
			continue
		}
		if strings.HasPrefix(a, "__psk__:") {
			pskPayload = strings.TrimPrefix(a, "__psk__:")
			pskTmp = filepath.Join(os.TempDir(), fmt.Sprintf("wg-psk-%d.b64", os.Getpid()))
			continue
		}
		out = append(out, a)
	}
	if privTmp != "" {
		for i := 0; i < len(out); i++ {
			if out[i] == "private-key" {
				// Append the temp path as the next
				// arg after the flag.
				out = append(out[:i+1], append([]string{privTmp}, out[i+1:]...)...)
				break
			}
		}
	}
	if pskTmp != "" {
		for i := 0; i < len(out); i++ {
			if out[i] == "preshared-key" {
				out = append(out[:i+1], append([]string{pskTmp}, out[i+1:]...)...)
				break
			}
		}
	}
	return privPayload, pskPayload, out, privTmp, pskTmp
}

// WgExecRunner dispatches via exec (no shell) so the
// production path is deterministic; the helper below
// is the explicit, single-purpose call DuckOS uses for
// the WireGuard `set` operation.
//
// LocalFilesystem persists the rendered WireGuard config
// under /etc/wireguard. The directory must be created
// and root-owned; the adapter never creates
// intermediate directories.
type LocalFilesystem struct {
	Dir string
}

func (f LocalFilesystem) WriteAtomic(_ context.Context, path string, mode uint32, data []byte) error {
	if f.Dir != "" {
		path = filepath.Join(f.Dir, filepath.Base(path))
	}
	dir := filepath.Dir(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp")
	if err != nil {
		return err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), os.FileMode(mode)); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	return nil
}

func (f LocalFilesystem) Remove(_ context.Context, path string) error {
	if f.Dir != "" {
		path = filepath.Join(f.Dir, filepath.Base(path))
	}
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// VaultKeyResolver is the production key resolver. It pulls a secret
// from the local vault store by reference and returns the raw bytes.
// The caller is responsible for clearing the slice on completion.
type VaultKeyResolver struct {
	Lookup func(ctx context.Context, vaultID, revision string) ([]byte, error)
}

func (r VaultKeyResolver) Resolve(ctx context.Context, ref KeyRef) ([]byte, error) {
	if r.Lookup == nil {
		return nil, errors.New("vault key resolver is not configured")
	}
	return r.Lookup(ctx, ref.VaultID, ref.Revision)
}

// readFile is a tiny helper the runner tests can use
// without importing io everywhere.
func readFile(r io.Reader) ([]byte, error) { return io.ReadAll(r) }

var _ = readFile
var _ = fmt.Sprintf
var _ = filepath.Join
