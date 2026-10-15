// nftlaunch/client.go: agent-side client for the privileged
// duck-nft-launcher.
//
// The client carries no privileged state: it dials the launcher's
// Unix socket, sends exactly one framed request, reads exactly one
// framed response and closes. Every mutation op hands the full nft
// batch to the launcher; the launcher — not this client — owns the
// ownership allowlist, the staging file, the nft exec and the
// timeout. A compromised agent can therefore only re-speak the same
// bounded protocol the real agent speaks.
package nftlaunch

import (
	"errors"
	"fmt"
	"net"
	"time"
)

// SocketPath returns the launcher socket path. Package-level var so
// tests can redirect to a temp socket.
var SocketPath = func() string { return DefaultSocket }

// Client-side timeouts. ExecTimeout bounds the whole round trip for
// mutation ops; the launcher itself enforces a shorter per-exec limit
// (ExecTimeout in server_linux.go), so a stuck nft cannot hold either
// side.
const (
	DialTimeout = 3 * time.Second
	RTTTimeout  = 30 * time.Second
	ExecTimeout = 20 * time.Second
)

// Client is a tiny one-shot launcher transport. It is safe for
// concurrent use (each call dials its own connection).
type Client struct {
	socketFn func() string
}

// NewClient returns a client bound to the canonical socket.
func NewClient() *Client { return &Client{socketFn: SocketPath} }

// SetSocketFn overrides the socket path (tests only).
func (c *Client) SetSocketFn(fn func() string) { c.socketFn = fn }

// Available probes the launcher without touching nft. The UPnP
// backend uses it instead of exec.LookPath("nft") so "backend
// available" means "the privileged path is up", which is the real
// precondition.
func (c *Client) Available() bool {
	resp, err := c.roundTrip(Request{Op: OpPing}, RTTTimeout)
	return err == nil && resp.Status == 200
}

// Check validates a batch with `nft --check` (no kernel mutation).
func (c *Client) Check(batch []byte) error {
	resp, err := c.roundTrip(Request{Op: OpCheck, Batch: batch}, ExecTimeout)
	return respErr(resp, err)
}

// CheckJSON is Check for an nft JSON document batch.
func (c *Client) CheckJSON(batch []byte) error {
	resp, err := c.roundTrip(Request{Op: OpCheck, Batch: batch, JSONInput: true}, ExecTimeout)
	return respErr(resp, err)
}

// Apply validates and atomically loads a batch.
func (c *Client) Apply(batch []byte) error {
	resp, err := c.roundTrip(Request{Op: OpApply, Batch: batch}, ExecTimeout)
	return respErr(resp, err)
}

// ApplyJSON is Apply for an nft JSON document batch.
func (c *Client) ApplyJSON(batch []byte) error {
	resp, err := c.roundTrip(Request{Op: OpApply, Batch: batch, JSONInput: true}, ExecTimeout)
	return respErr(resp, err)
}

// ListTableJSON returns `nft --json list table <family> <table>` for
// an owned table. The launcher refuses unknown tables with 403, so
// the error for a foreign table is a denial, not an nft failure.
func (c *Client) ListTableJSON(family, table string) ([]byte, error) {
	return c.readOp(Request{Op: OpListTable, Family: family, Table: table}, ExecTimeout)
}

// ListRulesetJSON returns `nft --json list ruleset`.
func (c *Client) ListRulesetJSON() ([]byte, error) {
	return c.readOp(Request{Op: OpListRuleset}, ExecTimeout)
}

func (c *Client) readOp(req Request, timeout time.Duration) ([]byte, error) {
	resp, err := c.roundTrip(req, timeout)
	if err := respErr(resp, err); err != nil {
		return nil, err
	}
	return resp.Output, nil
}

func (c *Client) roundTrip(req Request, timeout time.Duration) (Response, error) {
	var resp Response
	conn, err := net.DialTimeout("unix", c.socketFn(), DialTimeout)
	if err != nil {
		return resp, fmt.Errorf("nft launcher is unreachable: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if err := EncodeFrame(conn, req); err != nil {
		return resp, err
	}
	if err := DecodeFrame(conn, &resp); err != nil {
		return resp, fmt.Errorf("nft launcher response is unreadable: %w", err)
	}
	return resp, nil
}

func respErr(resp Response, err error) error {
	if err != nil {
		return err
	}
	if resp.Status == 200 {
		return nil
	}
	if resp.Message == "" {
		return fmt.Errorf("nft launcher refused op (status %d, code %s)", resp.Status, resp.Code)
	}
	return errors.New(resp.Message)
}
