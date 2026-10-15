//go:build linux

// cmd/duck-nft-launcher/main.go: privileged launcher for the
// DuckOS-owned nftables operations.
//
// BETA.3 privilege-boundary remediation. duck-agent runs with
// CapabilityBoundingSet= empty, AmbientCapabilities= empty and
// NoNewPrivileges=yes — an intentional security invariant. The
// firewall (inet duckos / inet duckos_fw) and UPnP (inet duckos_upnp)
// adapters nevertheless exec'd nft directly inside the agent, and
// every such invocation failed in production with "Operation not
// permitted" because the relevant netlink operations require
// CAP_NET_ADMIN. Root alone is not enough: the agent's effective
// capability set is empty.
//
// This launcher follows the established DuckOS privileged-execution
// pattern (duck-wg-launcher, duck-frr-launcher): a separate systemd
// unit holding exactly CAP_NET_ADMIN, a Unix socket in the agent's
// /run/duckos-agent directory, and SO_PEERCRED authentication on
// every connection. The agent keeps zero capabilities; the launcher
// is the single privilege holder for the bounded nftables verbs.
//
// The full serving core (authentication, ownership allowlist, batch
// bounds, exec limits) lives in internal/nftlaunch; this main only
// parses flags and runs the server. The nft binary path is a
// build/install-time flag with a fixed default — no caller can
// influence the executed path.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/CrazyDuckLab/DuckOS/internal/nftlaunch"
)

var (
	listenFlag = flag.String("listen", nftlaunch.DefaultSocket, "unix socket listen path")
	nftPathFlg = flag.String("nft", "/usr/sbin/nft", "path to the nft(8) binary")
	uidFlag    = flag.Int("agent-uid", 0, "UID of the duck-agent peer (must match SO_PEERCRED)")
)

func main() {
	flag.Parse()
	if err := nftlaunch.Serve(*listenFlag, *nftPathFlg, uint32(*uidFlag)); err != nil {
		fmt.Fprintf(os.Stderr, "duck-nft-launcher: %v\n", err)
		os.Exit(1)
	}
}
