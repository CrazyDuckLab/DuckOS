// duck backup command: render the G17 portable/selective
// restore bridge. Read-only; restore execution remains
// Agent-privileged.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"
)

const backupHelp = `duck backup — render the backup catalog / restore plan

Usage:
  duck backup                       list available backups
  duck backup --id <backupId>       render restore plan for one backup
  duck backup [--output json|table|text]

Examples:
  duck backup
  duck backup --id daily-2026-01-01 --output table
`

// backupListResponse mirrors the IPC envelope.
type backupListResponse struct {
	Version   int             `json:"version"`
	UpdatedAt time.Time       `json:"updatedAt"`
	Backups   []backupSummary `json:"backups"`
}

type backupSummary struct {
	BackupID       string    `json:"backupId"`
	Mode           string    `json:"mode"`
	CreatedAt      time.Time `json:"createdAt"`
	SizeBytes      int64     `json:"sizeBytes"`
	ComponentCount int       `json:"componentCount"`
	Portable       bool      `json:"portable"`
	SecretGated    bool      `json:"secretGated"`
	Hostname       string    `json:"hostname,omitempty"`
	Note           string    `json:"note,omitempty"`
}

// backupPlanResponse mirrors the IPC envelope.
type backupPlanResponse struct {
	Version   int                `json:"version"`
	UpdatedAt time.Time          `json:"updatedAt"`
	Plan      restorePlanSummary `json:"plan"`
}

type restorePlanSummary struct {
	BackupID         string    `json:"backupId"`
	Safe             bool      `json:"safe"`
	RequiresSecret   bool      `json:"requiresSecret"`
	Conflicts        []string  `json:"conflicts,omitempty"`
	Missing          []string  `json:"missing,omitempty"`
	Components       []string  `json:"components,omitempty"`
	EstimatedSeconds int       `json:"estimatedSeconds"`
	GeneratedAt      time.Time `json:"generatedAt"`
}

func backupCmd(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cf := addCommonFlags(fs)
	var id string
	fs.StringVar(&id, "id", "", "Optional backup id for restore plan")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Print(backupHelp)
			return nil
		}
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	path := "/api/v1/backup"
	if id != "" {
		path += "?id=" + id
	}
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: path}, nil)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("backup: HTTP %d", code)
	}
	if id == "" {
		var list backupListResponse
		if _, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: path}, &list); err != nil {
			return err
		}
		return renderBackupList(cf.output, &list)
	}
	var plan backupPlanResponse
	if _, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: path}, &plan); err != nil {
		return err
	}
	return renderBackupPlan(cf.output, &plan)
}

func renderBackupList(format string, r *backupListResponse) error {
	switch format {
	case "json", "":
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	case "text", "table":
		fmt.Printf("backup catalog version=%d updated=%s count=%d\n", r.Version, r.UpdatedAt.Format(time.RFC3339), len(r.Backups))
		for _, b := range r.Backups {
			fmt.Printf("  - %s mode=%s portable=%v secretGated=%v components=%d size=%d host=%s\n",
				b.BackupID, b.Mode, b.Portable, b.SecretGated, b.ComponentCount, b.SizeBytes, orDash(b.Hostname))
		}
		return nil
	}
	return nil
}

func renderBackupPlan(format string, r *backupPlanResponse) error {
	switch format {
	case "json", "":
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	case "text", "table":
		p := r.Plan
		fmt.Printf("restore plan version=%d backup=%s safe=%v requiresSecret=%v\n",
			r.Version, p.BackupID, p.Safe, p.RequiresSecret)
		if len(p.Conflicts) > 0 {
			fmt.Printf("  conflicts: %s\n", joinStrings(p.Conflicts, ","))
		}
		if len(p.Missing) > 0 {
			fmt.Printf("  missing:   %s\n", joinStrings(p.Missing, ","))
		}
		if len(p.Components) > 0 {
			fmt.Printf("  components: %s\n", joinStrings(p.Components, ","))
		}
		fmt.Printf("  estimated: %ds\n", p.EstimatedSeconds)
		return nil
	}
	return nil
}
