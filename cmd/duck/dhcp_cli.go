// dhcp_cli.go: the duck dhcp subcommand — read-only lease
// observation through the authenticated public API.
package main

import (
	"errors"
	"flag"
	"net/http"
)

func dhcpCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("dhcp: missing operation (leases)")
	}
	switch args[0] {
	case "leases":
		fs := flag.NewFlagSet("dhcp leases", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return authedCall(cf, http.MethodGet, "/api/v1/dhcp/leases", nil, "dhcp leases")
	default:
		return errors.New("dhcp: unknown operation " + args[0])
	}
}
