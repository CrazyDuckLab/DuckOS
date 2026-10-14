// wan_cli.go: the duck wan subcommand — staged WAN provider
// configuration and kernel observation.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
)

func wanCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("wan: missing operation (status | providers | stage | remove | apply)")
	}
	switch args[0] {
	case "status":
		fs := flag.NewFlagSet("wan status", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return authedCall(cf, http.MethodGet, "/api/v1/wan/status", nil, "wan status")
	case "providers":
		fs := flag.NewFlagSet("wan providers", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return authedCall(cf, http.MethodGet, "/api/v1/wan/providers", nil, "wan providers")
	case "stage":
		fs := flag.NewFlagSet("wan stage", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		file := fs.String("file", "", "Path to a provider JSON file")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *file == "" {
			return errors.New("wan stage: --file is required")
		}
		data, err := os.ReadFile(*file)
		if err != nil {
			return fmt.Errorf("read provider file: %w", err)
		}
		var provider map[string]any
		if err := json.Unmarshal(data, &provider); err != nil {
			return fmt.Errorf("provider file is not valid JSON: %w", err)
		}
		id, _ := provider["id"].(string)
		if id == "" {
			return errors.New("wan stage: provider id is required")
		}
		body, _ := json.Marshal(map[string]any{"provider": provider})
		return authedCall(cf, http.MethodPut, "/api/v1/wan/providers/"+id, body, "wan stage")
	case "remove":
		fs := flag.NewFlagSet("wan remove", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		id := fs.String("id", "", "Provider id to remove from staging")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *id == "" {
			return errors.New("wan remove: --id is required")
		}
		return authedCall(cf, http.MethodDelete, "/api/v1/wan/providers/"+*id, nil, "wan remove")
	case "apply":
		fs := flag.NewFlagSet("wan apply", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		id := fs.String("id", "", "Staged provider id to apply")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *id == "" {
			return errors.New("wan apply: --id is required")
		}
		fmt.Fprintln(os.Stderr, "⚠  WAN apply mutates a live uplink through the journal-first pipeline; it rolls back automatically unless confirmed (duck network confirm --tx). 🔴 РИСК: wrong uplink changes can drop management access.")
		body, _ := json.Marshal(map[string]string{"id": *id})
		return authedCall(cf, http.MethodPost, "/api/v1/wan/apply", body, "wan apply")
	default:
		return errors.New("wan: unknown operation " + args[0])
	}
}
