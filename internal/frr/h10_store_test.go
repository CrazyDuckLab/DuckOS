package frr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestH10DurableConfigAndNativeModeOrdering(t *testing.T) {
	dir := t.TempDir()
	m, err := OpenManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Daemons: []Daemon{DaemonZebra, DaemonBGP}, BGP: []BGPSession{{Name: "test", LocalAS: 65001, RouterID: "10.1.1.1", IPv4Unicast: true, Neighbors: []Neighbor{{Address: "10.1.1.2", RemoteAS: 65002}}, Networks: []Network{{Prefix: "192.0.2.0/24"}}}}}
	if err := m.Set(cfg); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := reopened.Render()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(rendered, "remote-as") > strings.Index(rendered, "address-family") {
		t.Fatal("remote-as emitted in address-family mode")
	}
	if !strings.Contains(rendered, "192.0.2.0/24") {
		t.Fatal("durable desired config lost")
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenManager(dir, nil); err == nil {
		t.Fatal("corrupt config accepted")
	}
}
