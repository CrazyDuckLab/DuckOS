//go:build linux

// nftlaunch/server_linux.go: the privileged side of the DuckOS
// nftables launcher. See cmd/duck-nft-launcher/main.go for the
// deployment story; this file holds the whole security-relevant
// serving core so tests (and the live Linux qualification suites)
// can run the launcher in-process against a real nft binary.
//
// Boundary summary:
//
//   - every connection is authenticated with SO_PEERCRED (agent UID);
//   - exactly one framed request per connection;
//   - unknown ops, unknown tables, unknown families, unknown verbs,
//     `flush ruleset`, define/include and oversized batches are
//     rejected before nft runs;
//   - nft is exec'd with fixed argv, a fixed PATH, a bounded
//     lifetime and bounded output; the batch is staged into a 0600
//     launcher-owned temp file (PrivateTmp=yes in the unit);
//   - concurrent nft invocations are serialised: every batch is an
//     atomic kernel transaction.

package nftlaunch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// ServerExecTimeout bounds one nft invocation. The launcher is the
// only clock that matters here; the client's deadline is a courtesy.
const ServerExecTimeout = 15 * time.Second

// Serve listens, sets the 0660 socket mode and serves until the
// listener closes. It is the production entry point and the test
// entry point (tests pass a temp socket).
func Serve(listen, nftPath string, agentUID uint32) error {
	if _, err := os.Stat(nftPath); err != nil {
		return fmt.Errorf("nft binary %q is not available: %v", nftPath, err)
	}
	if err := os.Remove(listen); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot clear stale socket: %v", err)
	}
	ln, err := net.Listen("unix", listen)
	if err != nil {
		return fmt.Errorf("listen: %v", err)
	}
	// 0660 on the socket, same access model as wg-launcher.sock: the
	// launcher and the agent both run as root:duckos, so a regular
	// local user cannot even connect.
	if err := os.Chmod(listen, 0o660); err != nil {
		ln.Close()
		return fmt.Errorf("chmod socket: %v", err)
	}
	return ServeListener(ln, nftPath, agentUID)
}

// ServeListener serves on an existing Unix listener. Exported so the
// Linux live-qualification tests can run the launcher in-process on
// a temp socket; production goes through Serve.
func ServeListener(ln net.Listener, nftPath string, agentUID uint32) error {
	var mu sync.Mutex // serialises nft execs: batches are atomic transactions
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			HandleConn(c, &mu, nftPath, agentUID)
		}(conn)
	}
}

// HandleConn authenticates the peer, then serves exactly one framed
// request per connection. One request per connection keeps the state
// machine trivial and the denial path total.
func HandleConn(conn net.Conn, mu *sync.Mutex, nftPath string, wantUID uint32) {
	_ = conn.SetDeadline(time.Now().Add(ExecTimeout + 5*time.Second))
	if err := assertPeerCred(conn, wantUID); err != nil {
		writeResp(conn, Response{Status: 401, Code: "unauthorized", Message: "peer is not the DuckOS agent"})
		return
	}
	var req Request
	if err := DecodeFrame(conn, &req); err != nil {
		writeResp(conn, Response{Status: 400, Code: "invalid_request", Message: "request frame is malformed or oversized"})
		return
	}
	writeResp(conn, ServeRequest(mu, nftPath, &req))
}

// ServeRequest dispatches one authenticated request. Unknown ops are
// rejected without touching nft.
func ServeRequest(mu *sync.Mutex, nftPath string, req *Request) Response {
	switch req.Op {
	case OpPing:
		return Response{Status: 200, Code: "ok"}
	case OpListTables, OpListRuleset:
		out, err := runNFT(mu, nftPath, listArgs(req.Op), nil, false)
		return readResp(out, err)
	case OpListTable:
		if err := ValidateReadTarget(req.Family, req.Table); err != nil {
			return Response{Status: 403, Code: "not_owned", Message: err.Error()}
		}
		out, err := runNFT(mu, nftPath, []string{"--json", "list", "table", req.Family, req.Table}, nil, false)
		return readResp(out, err)
	case OpCheck, OpApply:
		if err := ValidateBatch(req.Batch); err != nil {
			return Response{Status: 403, Code: "batch_rejected", Message: err.Error()}
		}
		// JSON input parses only under nft --json; text batches stay
		// in the default mode.
		var args []string
		if req.JSONInput {
			args = append(args, "--json")
		}
		args = append(args, "--file")
		var err error
		if req.Op == OpCheck {
			_, err = runNFT(mu, nftPath, append([]string{"--check"}, args...), req.Batch, true)
		} else {
			_, err = runNFT(mu, nftPath, args, req.Batch, true)
		}
		if err != nil {
			return Response{Status: 500, Code: "nft_failed", Message: redactNftError(err)}
		}
		return Response{Status: 200, Code: "ok"}
	default:
		return Response{Status: 400, Code: "unknown_op", Message: fmt.Sprintf("op %q is not supported by the launcher", string(req.Op))}
	}
}

func listArgs(op Op) []string {
	if op == OpListRuleset {
		return []string{"--json", "list", "ruleset"}
	}
	return []string{"--json", "list", "tables"}
}

// runNFT execs nft with fixed argv (never caller-supplied beyond the
// closed op set), staging the batch into a 0600 temp file the
// launcher owns. The mutex serialises nft invocations so two
// concurrent transactions cannot interleave.
func runNFT(mu *sync.Mutex, nftPath string, args []string, batch []byte, stage bool) ([]byte, error) {
	mu.Lock()
	defer mu.Unlock()
	if stage {
		f, err := os.CreateTemp("", "duckos-nft-*.nft")
		if err != nil {
			return nil, fmt.Errorf("stage write failed: %v", err)
		}
		tmp := f.Name()
		defer os.Remove(tmp)
		if err := f.Chmod(0o600); err != nil {
			f.Close()
			return nil, fmt.Errorf("stage chmod failed: %v", err)
		}
		if _, err := f.Write(batch); err != nil {
			f.Close()
			return nil, fmt.Errorf("stage write failed: %v", err)
		}
		if err := f.Close(); err != nil {
			return nil, fmt.Errorf("stage close failed: %v", err)
		}
		args = append(args, tmp)
	}
	ctx, cancel := context.WithTimeout(context.Background(), ServerExecTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, nftPath, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin", "LANG=C", "LC_ALL=C"}
	cmd.Dir = "/"
	var out strings.Builder
	var errOut strings.Builder
	bounded := &boundedWriter{max: MaxOutputBytes, buf: &out}
	cmd.Stdout = bounded
	cmd.Stderr = &errOut
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("nft execution exceeded the launcher time limit")
		}
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			return nil, fmt.Errorf("nft %s failed: %v", strings.Join(args, " "), err)
		}
		return nil, fmt.Errorf("nft failed: %v: %s", err, msg)
	}
	if bounded.overflow {
		return nil, errors.New("nft output exceeds the launcher size limit")
	}
	return []byte(out.String()), nil
}

// boundedWriter caps captured stdout.
type boundedWriter struct {
	max      int
	buf      *strings.Builder
	overflow bool
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	if w.overflow || w.buf.Len()+len(p) > w.max {
		w.overflow = true
		return 0, errors.New("nft output exceeds bound")
	}
	return w.buf.Write(p)
}

// redactNftError keeps the structured error short: nft stderr can
// quote offending rule lines, so the launcher relays only the error
// class plus the first 200 bytes.
func redactNftError(err error) string {
	s := err.Error()
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

func readResp(out []byte, err error) Response {
	if err != nil {
		return Response{Status: 500, Code: "nft_failed", Message: redactNftError(err)}
	}
	return Response{Status: 200, Code: "ok", Output: out}
}

func writeResp(conn net.Conn, resp Response) {
	_ = EncodeFrame(conn, resp)
}

// assertPeerCred is the same SO_PEERCRED gate the WG launcher uses.
func assertPeerCred(conn net.Conn, wantUID uint32) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return errors.New("non-unix peer rejected")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return err
	}
	var cred *unix.Ucred
	ctlErr := raw.Control(func(fd uintptr) {
		cred, err = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if ctlErr != nil {
		return ctlErr
	}
	if err != nil {
		return err
	}
	if cred.Uid != wantUID {
		return fmt.Errorf("peer UID %d rejected (want %d)", cred.Uid, wantUID)
	}
	return nil
}
