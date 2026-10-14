// firewall_cli.go: the duck firewall subcommand — read-only
// observation of the DuckOS-owned nftables table.
package main

import (
	"errors"
	"flag"
	"net/http"
)

func firewallCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("firewall: missing operation (rules)")
	}
	switch args[0] {
	case "rules":
		fs := flag.NewFlagSet("firewall rules", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return authedCall(cf, http.MethodGet, "/api/v1/firewall/rules", nil, "firewall rules")
	default:
		return errors.New("firewall: unknown operation " + args[0])
	}
}
