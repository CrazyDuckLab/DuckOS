// duck provenance command: render the G08 dependency /
// ownership / provenance view for a single object via the
// shared cliState pattern. The CLI and API go through the
// same Engine-mediated IPC, so there is no hidden second
// ownership surface.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

const provenanceHelp = `duck provenance — render ownership + provenance for one object

Usage:
  duck provenance --kind <kind> --id <id> [--output json|table|text]

Examples:
  duck provenance --kind vlan --id vlan-10
  duck provenance --kind xray-server --id primary --output table
`

// provenanceResponse mirrors the IPC envelope so the CLI
// can decode without importing internal/agent.
type provenanceResponse struct {
	Version   int                  `json:"version"`
	UpdatedAt time.Time            `json:"updatedAt"`
	Ownership *provenanceOwnership `json:"ownership,omitempty"`
	History   []provenanceEvent    `json:"history,omitempty"`
}

type provenanceOwnership struct {
	Generator string    `json:"generator,omitempty"`
	Manager   string    `json:"manager,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Source    string    `json:"source,omitempty"`
	FirstSeen time.Time `json:"firstSeen"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type provenanceEvent struct {
	Kind     string    `json:"kind"`
	Actor    string    `json:"actor,omitempty"`
	Reason   string    `json:"reason,omitempty"`
	Revision string    `json:"revision,omitempty"`
	At       time.Time `json:"at"`
}

func provenanceCmd(args []string) error {
	fs := flag.NewFlagSet("provenance", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cf := addCommonFlags(fs)
	var kind, id string
	fs.StringVar(&kind, "kind", "", "Object kind (e.g. vlan, wan, xray-server)")
	fs.StringVar(&id, "id", "", "Object id")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Print(provenanceHelp)
			return nil
		}
		return err
	}
	if kind == "" || id == "" {
		fmt.Print(provenanceHelp)
		return errors.New("provenance requires --kind and --id")
	}
	if err := validateProvenanceID(id); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	q := url.Values{}
	q.Set("kind", kind)
	q.Set("id", id)
	path := "/agent/v1/provenance?" + q.Encode()
	var out provenanceResponse
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: path}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("provenance: HTTP %d", code)
	}
	return renderProvenance(cf.output, &out)
}

func validateProvenanceID(s string) error {
	if !idPattern.MatchString(strings.TrimSpace(s)) {
		return errors.New("invalid --id: must match ^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$")
	}
	return nil
}

func renderProvenance(format string, r *provenanceResponse) error {
	switch format {
	case "json", "":
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	case "text", "table":
		return renderProvenanceText(r)
	}
	return nil
}

func renderProvenanceText(r *provenanceResponse) error {
	fmt.Printf("provenance version=%d updated=%s\n", r.Version, r.UpdatedAt.Format(time.RFC3339))
	if r.Ownership != nil {
		fmt.Println("ownership:")
		fmt.Printf("  generator: %s\n", orDash(r.Ownership.Generator))
		fmt.Printf("  manager:   %s\n", orDash(r.Ownership.Manager))
		fmt.Printf("  source:    %s\n", orDash(r.Ownership.Source))
		if len(r.Ownership.Tags) > 0 {
			fmt.Printf("  tags:      %s\n", strings.Join(r.Ownership.Tags, ","))
		}
		fmt.Printf("  firstSeen: %s\n", r.Ownership.FirstSeen.Format(time.RFC3339))
		fmt.Printf("  updatedAt: %s\n", r.Ownership.UpdatedAt.Format(time.RFC3339))
	} else {
		fmt.Println("ownership: <none>")
	}
	if len(r.History) == 0 {
		fmt.Println("history: <none>")
		return nil
	}
	fmt.Println("history:")
	for _, ev := range r.History {
		fmt.Printf("  - %s @ %s actor=%s reason=%s revision=%s\n",
			ev.Kind, ev.At.Format(time.RFC3339),
			orDash(ev.Actor), orDash(ev.Reason), orDash(ev.Revision))
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
