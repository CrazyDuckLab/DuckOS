package network

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"syscall"
	"time"
)

// Enrollment is local root authority, separate from API desired state. A
// logical ID/name alone never grants ownership of a physical NIC or VLAN name.
// Daemon/file conflict detection remains an additional activation gate.
type Enrollment struct {
	Version        int               `json:"version"`
	CreatedAt      time.Time         `json:"createdAt"`
	Physical       []PhysicalBinding `json:"physical"`
	VLANNames      []string          `json:"vlanNames"`
	FirewallTable  string            `json:"firewallTable"`
	RoutingTables  []int             `json:"routingTables,omitempty"`
	PolicyRouting  bool              `json:"policyRouting,omitempty"`
	NetworkdReload bool              `json:"networkdReload,omitempty"`
}
type PhysicalBinding struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	MAC  string `json:"mac"`
	// ExternallyManaged marks a Physical binding that the host keeps under
	// another network manager (typically dhcpcd for the management NIC).
	// The Agent owns the durable identity for cross-manager accounting but
	// never writes a networkd profile, replaces an existing file, or reloads
	// networkd against this link. Explicit mutations to an externally
	// managed binding are rejected in the adapter Preflight path so the
	// management plane cannot be claimed by a stray config/apply.
	ExternallyManaged bool `json:"externallyManaged,omitempty"`
}

func DecodeEnrollment(raw []byte) (Enrollment, error) {
	var e Enrollment
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 64<<10 || d.Decode(&e) != nil || d.Decode(new(any)) != io.EOF {
		return e, errors.New("invalid ownership enrollment document")
	}
	return e, e.Validate()
}

func (e Enrollment) Validate() error {
	if e.Version != 1 || e.CreatedAt.IsZero() || len(e.Physical) == 0 || len(e.Physical) > 64 || len(e.VLANNames) > 64 || len(e.RoutingTables) > 128 || e.FirewallTable != "inet duckos" {
		return errors.New("invalid enrollment scope or version")
	}
	ids, names, macs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, b := range e.Physical {
		if !identifier.MatchString(b.ID) || ids[b.ID] || !linkName.MatchString(b.Name) || b.Name == "lo" || names[b.Name] || !unicastMAC(b.MAC) || macs[b.MAC] {
			return errors.New("ambiguous physical ownership binding")
		}
		ids[b.ID], names[b.Name], macs[b.MAC] = true, true, true
	}
	for _, name := range e.VLANNames {
		if !linkName.MatchString(name) || name == "lo" || names[name] {
			return errors.New("ambiguous VLAN ownership binding")
		}
		names[name] = true
	}
	tables := map[int]bool{}
	for _, table := range e.RoutingTables {
		if !privateRouteTable(table) || tables[table] {
			return errors.New("invalid or duplicate private routing table enrollment")
		}
		tables[table] = true
	}
	return nil
}

// CheckBindings uses a fresh, independently captured Linux snapshot. Interface
// indices may change across boots; root-enrolled ID/name/MAC must still agree.
// This does not attest that networkd owns the link or that it is reachable.
func (e Enrollment) CheckBindings(before, next Config, live HostSnapshot) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if err := before.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(live)
	if err != nil {
		return err
	}
	if _, err := DecodeHostSnapshot(raw, before, next); err != nil {
		return err
	}
	bindings := map[string]PhysicalBinding{}
	for _, b := range e.Physical {
		bindings[b.ID] = b
	}
	vlans := map[string]bool{}
	for _, name := range e.VLANNames {
		vlans[name] = true
	}
	actual := map[string]string{}
	for _, link := range live.Links {
		actual[link.Name] = link.MAC
	}
	for _, c := range []Config{before, next} {
		allowedTables := map[int]bool{}
		for _, table := range e.RoutingTables {
			allowedTables[table] = true
		}
		for _, route := range c.Routes {
			if privateRouteTable(route.Table) && !allowedTables[route.Table] {
				return errors.New("private routing table is not root-enrolled")
			}
		}
		if len(c.PolicyRules) > 0 && !e.PolicyRouting {
			return errors.New("routing policy priority ranges are not root-enrolled")
		}
		for _, link := range c.Interfaces {
			if link.Kind == "vlan" {
				if !vlans[link.Name] {
					return errors.New("VLAN name not enrolled")
				}
				continue
			}
			b, ok := bindings[link.ID]
			if !ok || b.Name != link.Name || b.MAC != actual[link.Name] {
				return errors.New("physical binding differs from root enrollment")
			}
		}
	}
	return nil
}

// ReadEnrollment reads one fixed basename from an existing private Agent
// directory. It never creates, upgrades or overwrites operator enrollment.
func ReadEnrollment(directory string) (Enrollment, error) {
	info, err := os.Lstat(directory)
	if err != nil || !privateEnrollmentOwner(info, true) {
		return Enrollment{}, errors.New("unsafe enrollment directory")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return Enrollment{}, err
	}
	defer root.Close()
	info, err = root.Lstat("enrollment.json")
	if err != nil || !privateEnrollmentOwner(info, false) || info.Size() > 64<<10 {
		return Enrollment{}, errors.New("missing or unsafe enrollment file")
	}
	f, err := root.OpenFile("enrollment.json", os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return Enrollment{}, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || !privateEnrollmentOwner(opened, false) {
		return Enrollment{}, errors.New("enrollment changed while opening")
	}
	raw, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return Enrollment{}, err
	}
	return DecodeEnrollment(raw)
}

func privateEnrollmentOwner(info os.FileInfo, directory bool) bool {
	if info == nil {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Geteuid() {
		return false
	}
	if directory {
		return info.IsDir() && info.Mode().Perm() == 0700
	}
	return info.Mode().IsRegular() && info.Mode().Perm() == 0600 && st.Nlink == 1
}
