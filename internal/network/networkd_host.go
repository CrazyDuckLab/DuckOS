package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"runtime"
	"time"
)

type NetworkdLink struct {
	Index               int      `json:"Index"`
	Name                string   `json:"Name"`
	Kind                string   `json:"Kind"`
	NetworkFile         string   `json:"NetworkFile"`
	NetworkFileDropins  []string `json:"NetworkFileDropins"`
	AdministrativeState string   `json:"AdministrativeState"`
}

// DecodeNetworkdInventory extracts only ownership evidence, not networkd's
// private leases or unrelated metadata. Unknown systemd fields are permitted;
// missing inventory, duplicate identities and malformed selected fields are not.
func DecodeNetworkdInventory(raw []byte) ([]NetworkdLink, error) {
	var wire struct {
		Interfaces *[]NetworkdLink `json:"Interfaces"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if len(raw) > 2<<20 || d.Decode(&wire) != nil || d.Decode(new(any)) != io.EOF || wire.Interfaces == nil || len(*wire.Interfaces) > 256 {
		return nil, errors.New("invalid networkd ownership inventory")
	}
	names, indices := map[string]bool{}, map[int]bool{}
	for _, link := range *wire.Interfaces {
		if link.Name == "" || len(link.Name) > 15 || names[link.Name] || link.Index <= 0 || indices[link.Index] || len(link.NetworkFile) > 4096 || len(link.NetworkFileDropins) > 64 || len(link.AdministrativeState) > 64 || len(link.Kind) > 64 {
			return nil, errors.New("ambiguous networkd link inventory")
		}
		names[link.Name], indices[link.Index] = true, true
	}
	return *wire.Interfaces, nil
}

// CheckNetworkdOwnership rejects competing winning profiles and drop-ins.
// A new VLAN must not already exist; a new physical NIC must be unmanaged by
// networkd. NIC enrollment, artifact byte checks and other managers are separate
// gates, so this function alone never authorizes takeover or mutation.
func CheckNetworkdOwnership(before, next Config, inventory []NetworkdLink) error {
	if err := before.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	live := map[string]NetworkdLink{}
	for _, link := range inventory {
		if _, exists := live[link.Name]; exists {
			return errors.New("duplicate networkd identity")
		}
		live[link.Name] = link
	}
	owned := map[string]Interface{}
	for _, link := range before.Interfaces {
		actual, ok := live[link.Name]
		if !ok || actual.NetworkFile != "/etc/systemd/network/10-duckos-"+link.ID+".network" || len(actual.NetworkFileDropins) != 0 || actual.AdministrativeState != "configured" {
			return errors.New("managed interface has missing, failed or foreign networkd profile")
		}
		if (link.Kind == "vlan" && actual.Kind != "vlan") || (link.Kind == "physical" && actual.Kind != "") {
			return errors.New("networkd link kind differs from intent")
		}
		owned[link.Name] = link
	}
	for _, link := range next.Interfaces {
		if previous, exists := owned[link.Name]; exists {
			if previous.ID != link.ID || previous.Kind != link.Kind || previous.Parent != link.Parent || previous.VLAN != link.VLAN {
				return errors.New("kernel link identity change requires explicit replacement plan")
			}
			continue
		}
		actual, exists := live[link.Name]
		if link.Kind == "vlan" {
			if exists {
				return errors.New("new VLAN collides with existing kernel link")
			}
			continue
		}
		if !exists || actual.Kind != "" || actual.NetworkFile != "" || len(actual.NetworkFileDropins) != 0 || actual.AdministrativeState != "unmanaged" {
			return errors.New("new physical interface is not an unmanaged enrollment candidate")
		}
	}
	return nil
}

// ReadNetworkdInventory uses the systemd 257 manager Describe JSON returned by
// networkctl list (not the human-oriented status output). No shell or mutation.
func ReadNetworkdInventory(parent context.Context) ([]NetworkdLink, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("networkd inventory requires Linux")
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/networkctl", "--no-pager", "--no-ask-password", "--json=short", "list")
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin", "LANG=C", "LC_ALL=C", "SYSTEMD_COLORS=0"}
	cmd.Dir = "/"
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	var out boundedNetworkdOutput
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, errors.New("networkd inventory unavailable or exceeded limits")
	}
	return DecodeNetworkdInventory(out.Bytes())
}

type boundedNetworkdOutput struct{ bytes.Buffer }

func (b *boundedNetworkdOutput) Write(p []byte) (int, error) {
	if len(p) > (2<<20)-b.Len() {
		return 0, errors.New("networkd output exceeds limit")
	}
	return b.Buffer.Write(p)
}
