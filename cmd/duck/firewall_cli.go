// firewall_cli.go: the duck firewall subcommand — typed rule
// management plus read-only observation of the owned table.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
)

func firewallCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("firewall: missing operation (status | rules | observed | add | update | delete | disable | enable | confirm | rollback)")
	}
	switch args[0] {
	case "status":
		fs := flag.NewFlagSet("firewall status", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return authedCall(cf, http.MethodGet, "/api/v1/firewall/rules/status", nil, "firewall status")
	case "confirm", "rollback":
		fs := flag.NewFlagSet("firewall "+args[0], flag.ContinueOnError)
		cf := addCommonFlags(fs)
		tx := fs.String("tx", "", "Transaction id to confirm or roll back")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *tx == "" {
			return errors.New("firewall " + args[0] + ": --tx is required")
		}
		body, _ := json.Marshal(map[string]string{"transactionId": *tx})
		return authedCall(cf, http.MethodPost, "/api/v1/firewall/rules/"+args[0], body, "firewall "+args[0])
	case "rules":
		fs := flag.NewFlagSet("firewall rules", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return authedCall(cf, http.MethodGet, "/api/v1/firewall/rules", nil, "firewall rules")
	case "observed":
		fs := flag.NewFlagSet("firewall observed", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return authedCall(cf, http.MethodGet, "/api/v1/firewall/rules/observed", nil, "firewall observed")
	case "add", "update":
		fs := flag.NewFlagSet("firewall "+args[0], flag.ContinueOnError)
		cf := addCommonFlags(fs)
		file := fs.String("file", "", "Path to a rule JSON file")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *file == "" {
			return errors.New("firewall add: --file is required")
		}
		data, err := os.ReadFile(*file)
		if err != nil {
			return fmt.Errorf("read rule file: %w", err)
		}
		var rule map[string]any
		if err := json.Unmarshal(data, &rule); err != nil {
			return fmt.Errorf("rule file is not valid JSON: %w", err)
		}
		id, _ := rule["id"].(string)
		if id == "" {
			return errors.New("firewall add: rule id is required")
		}
		body, _ := json.Marshal(map[string]any{"rule": rule})
		return authedCall(cf, http.MethodPut, "/api/v1/firewall/rules/"+id, body, "firewall "+args[0])
	case "delete":
		fs := flag.NewFlagSet("firewall delete", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		id := fs.String("id", "", "Rule id to delete")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *id == "" {
			return errors.New("firewall delete: --id is required")
		}
		return authedCall(cf, http.MethodDelete, "/api/v1/firewall/rules/"+*id, nil, "firewall delete")
	case "enable", "disable":
		fs := flag.NewFlagSet("firewall "+args[0], flag.ContinueOnError)
		cf := addCommonFlags(fs)
		id := fs.String("id", "", "Rule id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *id == "" {
			return errors.New("firewall " + args[0] + ": --id is required")
		}
		body := `{"enabled":true}`
		if args[0] == "disable" {
			body = `{"enabled":false}`
		}
		return authedCall(cf, http.MethodPut, "/api/v1/firewall/rules/"+*id+"/enabled", []byte(body), "firewall "+args[0])
	default:
		return errors.New("firewall: unknown operation " + args[0])
	}
}
