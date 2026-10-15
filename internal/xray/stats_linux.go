// Package xray / stats_linux.go: production stats collector. The
// stats service is queried through the bundled xray binary itself
// (`xray api statsquery`) rather than a hand-rolled gRPC client:
// the binary is already pinned, sha256-verified and present at the
// known install path, so the subprocess keeps the wire protocol in
// Xray-core's own hands. Output is parsed by ParseStatsQueryOutput.
package xray

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// StatsAPIAddr is the default loopback dokodemo-door address the
// agent renders for the stats service when stats are enabled.
const StatsAPIAddr = "127.0.0.1:10085"

// StatsCollectorOptions parameterise the subprocess collector.
type StatsCollectorOptions struct {
	// Bin is the xray binary path (defaults to DUCKOS_XRAY or
	// /usr/local/bin/xray via the runtime's resolution).
	Bin string
	// Server is the stats API address (default StatsAPIAddr).
	Server string
	// Timeout bounds one collection round.
	Timeout time.Duration
}

// NewSubprocessStatsCollector returns a Collector running
// `xray api statsquery --server=<addr>`. The output cap mirrors the
// agent's IPC cap; a hostile counter label cannot exceed it.
func NewSubprocessStatsCollector(opts StatsCollectorOptions) Collector {
	if opts.Server == "" {
		opts.Server = StatsAPIAddr
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	return func(ctx context.Context) (StatsSnapshot, error) {
		bin := opts.Bin
		if bin == "" {
			bin = XrayPath
		}
		cctx, cancel := context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
		cmd := exec.CommandContext(cctx, bin, "api", "statsquery", "--server="+opts.Server)
		// Bounded environment, mirroring the runtime launcher.
		cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin"}
		out, err := cmd.Output()
		if err != nil {
			return StatsSnapshot{}, fmt.Errorf("%w: %v", ErrStatsUnavailable, err)
		}
		if len(out) > 4<<20 {
			return StatsSnapshot{}, fmt.Errorf("xray: statsquery output exceeds cap")
		}
		snap, err := ParseStatsQueryOutput(out)
		if err != nil {
			return StatsSnapshot{}, err
		}
		return snap, nil
	}
}
