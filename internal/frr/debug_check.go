//go:build debug

package frr

import (
	"encoding/json"
	"fmt"
)

func DebugPrintConfig() {
	cfg := Config{
		Daemons: []Daemon{DaemonZebra, DaemonBGP},
		BGP: []BGPSession{{
			Name:              "duck-dv09-1",
			LocalAS:           65001,
			RouterID:          "10.250.100.1",
			Networks:          []Network{{Prefix: "192.0.2.0/24"}},
			Neighbors:         []Neighbor{{Address: "10.250.0.2", RemoteAS: 65002}},
			IPv4Unicast:       true,
			StandardCommunity: true,
		}},
	}
	if err := cfg.Valid(); err != nil {
		fmt.Println("Valid error:", err)
		return
	}
	rendered, err := Render(cfg)
	if err != nil {
		fmt.Println("Render error:", err)
		return
	}
	fmt.Println("--- rendered ---")
	fmt.Println(rendered)
	b, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println("--- json ---")
	fmt.Println(string(b))
}
