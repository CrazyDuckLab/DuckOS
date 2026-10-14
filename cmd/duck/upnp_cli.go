// upnp_cli.go: the duck upnp subcommand — a thin client of
// the public /api/v1/upnp surface. State, enable/disable of
// trusted networks, and mapping revoke. The CLI never talks
// to the Agent socket for these operations: the public API
// path is the same one the Web UI uses, so authorization and
// audit stay identical.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"strings"
)

func upnpCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("upnp: missing operation (state | enable | disable | revoke)")
	}
	op, rest := args[0], args[1:]
	switch op {
	case "state":
		fs := flag.NewFlagSet("upnp state", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		if err := fs.Parse(rest); err != nil {
			return err
		}
		return authedCall(cf, http.MethodGet, "/api/v1/upnp/state", nil, "upnp state")
	case "enable":
		return upnpEnable(rest)
	case "disable":
		fs := flag.NewFlagSet("upnp disable", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		network := fs.String("network", "", "Trusted network ref to disable")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if *network == "" {
			return errors.New("upnp disable: --network is required")
		}
		return authedCall(cf, http.MethodDelete, "/api/v1/upnp/networks/"+*network, nil, "upnp disable")
	case "revoke":
		fs := flag.NewFlagSet("upnp revoke", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		id := fs.String("id", "", "Mapping id to revoke")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if *id == "" {
			return errors.New("upnp revoke: --id is required")
		}
		return authedCall(cf, http.MethodDelete, "/api/v1/upnp/mappings/"+*id, nil, "upnp revoke")
	default:
		return fmt.Errorf("upnp: unknown operation %q", op)
	}
}

func upnpEnable(rest []string) error {
	fs := flag.NewFlagSet("upnp enable", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	network := fs.String("network", "", "Trusted network ref to enable")
	iface := fs.String("iface", "", "Kernel interface name the ref resolves to")
	protocols := fs.String("protocols", "pcp,nat-pmp,upnp", "Comma-separated protocols: pcp,nat-pmp,upnp")
	ttl := fs.Int("ttl", 3600, "Maximum mapping lifetime in seconds")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if *network == "" {
		return errors.New("upnp enable: --network is required")
	}
	var protos []map[string]string
	for _, p := range strings.Split(*protocols, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		switch p {
		case "pcp", "nat-pmp", "upnp":
			protos = append(protos, map[string]string{"p": p})
		default:
			return fmt.Errorf("upnp enable: unknown protocol %q", p)
		}
	}
	if len(protos) == 0 {
		return errors.New("upnp enable: at least one protocol is required")
	}
	protoNames := make([]string, 0, len(protos))
	for _, p := range protos {
		protoNames = append(protoNames, p["p"])
	}
	body, err := json.Marshal(map[string]any{
		"network": map[string]any{
			"networkRef": *network,
			"protocols":  protoNames,
			"ttlSec":     *ttl,
		},
		"iface": *iface,
	})
	if err != nil {
		return err
	}
	return authedCall(cf, http.MethodPut, "/api/v1/upnp/networks/"+*network, body, "upnp enable")
}
