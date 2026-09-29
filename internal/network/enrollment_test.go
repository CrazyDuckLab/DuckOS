package network

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func syntheticEnrollment() Enrollment {
	return Enrollment{Version: 1, CreatedAt: time.Now().UTC(), FirewallTable: "inet duckos", Physical: []PhysicalBinding{
		{ID: "uplink", Name: "testwan0", MAC: "02:00:00:00:00:01"},
		{ID: "inside", Name: "testlan0", MAC: "02:00:00:00:00:02"},
	}, VLANNames: []string{"testguest0"}}
}

func TestEnrollmentBindsPhysicalIdentityAndReservedVLANNames(t *testing.T) {
	e := syntheticEnrollment()
	c := fixture()
	live := HostSnapshot{Version: 1, CapturedAt: time.Now().UTC(), Links: []HostLink{
		{Name: "testwan0", Index: 2, MAC: e.Physical[0].MAC, MTU: 1500},
		{Name: "testlan0", Index: 3, MAC: e.Physical[1].MAC, MTU: 1500},
	}}
	if err := e.CheckBindings(c, c, live); err != nil {
		t.Fatal(err)
	}
	live.Links[0].Index = 17
	if err := e.CheckBindings(c, c, live); err != nil {
		t.Fatal("boot index change rejected", err)
	}
	live.Links[0].MAC = "02:00:00:00:00:99"
	if err := e.CheckBindings(c, c, live); err == nil {
		t.Fatal("replacement NIC silently adopted")
	}
	live.Links[0].MAC = e.Physical[0].MAC
	next := fixture()
	next.Interfaces = append(next.Interfaces, Interface{ID: "guest", Name: "testguest0", Kind: "vlan", Parent: "inside", VLAN: 20, MTU: 1500, Role: "lan", DHCPClient: "none", Addresses: []string{"203.0.113.1/24"}})
	if err := e.CheckBindings(c, next, live); err != nil {
		t.Fatal(err)
	}
	next.Interfaces[2].Name = "foreignvlan0"
	if err := e.CheckBindings(c, next, live); err == nil {
		t.Fatal("unenrolled VLAN name adopted")
	}
}

func TestEnrollmentDocumentAndPrivateFileBoundary(t *testing.T) {
	e := syntheticEnrollment()
	raw, _ := json.Marshal(e)
	if _, err := DecodeEnrollment(raw); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{[]byte(`{}`), append(append([]byte(nil), raw...), []byte(` {}`)...), []byte(`{"version":1,"command":"anything"}`)} {
		if _, err := DecodeEnrollment(bad); err == nil {
			t.Fatal("invalid document admitted")
		}
	}
	for name, mutate := range map[string]func(Enrollment) Enrollment{
		"foreign table":  func(e Enrollment) Enrollment { e.FirewallTable = "inet foreign"; return e },
		"duplicate name": func(e Enrollment) Enrollment { e.VLANNames = []string{"testwan0"}; return e },
		"duplicate MAC":  func(e Enrollment) Enrollment { e.Physical[1].MAC = e.Physical[0].MAC; return e },
	} {
		t.Run(name, func(t *testing.T) {
			if mutate(syntheticEnrollment()).Validate() == nil {
				t.Fatal("ambiguous enrollment admitted")
			}
		})
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "enrollment.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEnrollment(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEnrollment(dir); err == nil {
		t.Fatal("public enrollment accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEnrollment(dir); err == nil {
		t.Fatal("hard-linked enrollment accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(alias, path); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEnrollment(dir); err == nil {
		t.Fatal("symbolic enrollment accepted")
	}
	after, err := os.ReadFile(alias)
	if err != nil || string(after) != string(raw) {
		t.Fatal("reader modified root enrollment")
	}
}
