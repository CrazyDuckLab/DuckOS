package main

import (
	"errors"
	"fmt"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"os"
)

func ownerCmd(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("Usage: sudo duck owner reset\nStop duck-api first; start it after recovery and enroll using the installation bootstrap credential. Removes Owner accounts only, preserves other users. Backups: /var/backups/duckos-owner. Never restarts network services.")
		return nil
	}
	if os.Geteuid() != 0 {
		return errors.New("Owner recovery requires effective root")
	}
	if len(args) != 1 || args[0] != "reset" {
		return errors.New("usage: sudo duck owner reset")
	}
	backup, err := identity.RecoverOwner("/var/lib/duckos/identity", "/var/backups/duckos-owner")
	if backup != "" {
		fmt.Println("Identity backup:", backup)
	}
	if err != nil {
		return err
	}
	fmt.Println("Owner recovery committed. Start duck-api and complete normal Owner enrollment. All previous sessions are invalid.")
	return nil
}
