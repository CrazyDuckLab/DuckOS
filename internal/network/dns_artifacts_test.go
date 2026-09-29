package network

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDNSArtifactRoleAndRecovery(t *testing.T) {
	path := PrivateTempDir(t)
	d, err := OpenDNSArtifacts(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	before := map[string]string{"dnsmasq.conf": "# synthetic old\nport=53\n"}
	next := map[string]string{"dnsmasq.conf": "# synthetic next\nport=53\n"}
	if err = d.Replace(nil, before); err != nil {
		t.Fatal(err)
	}
	if err = d.Replace(before, next); err != nil {
		t.Fatal(err)
	}
	if err = d.Restore(before, next); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(path, "dnsmasq.conf"))
	if err != nil || string(data) != before["dnsmasq.conf"] {
		t.Fatal("DNS recovery lost old configuration", err)
	}
	for _, name := range []string{"../dnsmasq.conf", "other.conf", "10-duckos-inside.network"} {
		if err = d.Check(before, map[string]string{name: "arbitrary"}); err == nil {
			t.Fatal("DNS role accepted unrelated artifact", name)
		}
	}
	if err = d.Replace(before, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(path, "dnsmasq.conf")); !os.IsNotExist(err) {
		t.Fatal("disabled DNS configuration left behind")
	}
}
