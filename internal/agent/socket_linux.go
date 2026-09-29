//go:build linux

package agent

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type peerListener struct {
	*net.UnixListener
	uid uint32
}

func (l *peerListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.AcceptUnix()
		if err != nil {
			return nil, err
		}
		if authenticatedPeer(conn, l.uid) == nil {
			return conn, nil
		}
		_ = conn.Close()
	}
}
func authenticatedPeer(conn *net.UnixConn, allowed uint32) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var credential *syscall.Ucred
	var inner error
	if err = raw.Control(func(fd uintptr) {
		credential, inner = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return err
	}
	if inner != nil {
		return inner
	}
	if credential == nil || credential.Uid != allowed {
		return errors.New("unauthorized peer")
	}
	return nil
}

func Listen(path string, allowedUID uint32) (net.Listener, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > 100 {
		return nil, errors.New("socket must be a clean absolute path of at most 100 bytes")
	}
	parent, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	owner, ok := parent.Sys().(*syscall.Stat_t)
	if !parent.IsDir() || parent.Mode().Perm()&0022 != 0 || !ok || owner.Uid != uint32(os.Geteuid()) {
		return nil, errors.New("socket directory must be owned by Agent identity and not writable by group or others")
	}
	if existing, err := os.Lstat(path); err == nil {
		st, ok := existing.Sys().(*syscall.Stat_t)
		if existing.Mode()&os.ModeSocket == 0 || !ok || st.Uid != uint32(os.Geteuid()) {
			return nil, errors.New("refusing to replace non-owned socket path")
		}
		conn, dialErr := net.DialTimeout("unix", path, 100*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			return nil, errors.New("Agent socket is already active")
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("cannot establish stale socket: %w", dialErr)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0660); err != nil {
		_ = listener.Close()
		return nil, err
	}
	return &peerListener{UnixListener: listener, uid: allowedUID}, nil
}
