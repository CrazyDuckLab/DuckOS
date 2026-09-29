package network

import (
	"os"
	"path/filepath"
	"testing"
)

func artifactTest(t *testing.T) (*ArtifactDirectory, string) {
	t.Helper()
	path := t.TempDir()
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
	d, err := OpenArtifactDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d, path
}
func TestArtifactReplaceRestoreAndUnknownPreservation(t *testing.T) {
	d, path := artifactTest(t)
	name := "10-duckos-inside.network"
	old := map[string]string{name: "[Match]\nName=test0\n"}
	next := map[string]string{name: "[Match]\nName=test1\n", "10-duckos-guest.netdev": "[NetDev]\nName=test2\nKind=vlan\n"}
	unknown := filepath.Join(path, "90-unmanaged.network")
	if err := os.WriteFile(unknown, []byte("do not touch"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := d.Replace(nil, old); err != nil {
		t.Fatal(err)
	}
	if err := d.Replace(old, next); err != nil {
		t.Fatal(err)
	}
	if err := d.Restore(old, next); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(path, name))
	if err != nil || string(got) != old[name] {
		t.Fatal("before state lost")
	}
	if _, err = os.Stat(filepath.Join(path, "10-duckos-guest.netdev")); !os.IsNotExist(err) {
		t.Fatal("introduced VLAN artifact retained")
	}
	got, _ = os.ReadFile(unknown)
	if string(got) != "do not touch" {
		t.Fatal("unmanaged file changed")
	}
	info, _ := os.Stat(filepath.Join(path, name))
	if info.Mode().Perm() != 0644 {
		t.Fatal("networkd cannot read generated config")
	}
}
func TestArtifactsRejectDriftCollisionsAndUnsafePaths(t *testing.T) {
	d, path := artifactTest(t)
	name := "10-duckos-inside.network"
	old := map[string]string{name: "old"}
	next := map[string]string{name: "new"}
	if err := d.Replace(nil, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, name), []byte("manual root change"), 0644); err != nil {
		t.Fatal(err)
	}
	if d.Replace(old, next) == nil || d.Restore(old, next) == nil {
		t.Fatal("manual drift overwritten")
	}
	got, _ := os.ReadFile(filepath.Join(path, name))
	if string(got) != "manual root change" {
		t.Fatal("drift evidence lost")
	}
	if d.Replace(nil, old) == nil {
		t.Fatal("adopted existing file")
	}
	for _, bad := range []string{"../escape", "10-duckos-../escape.network", "10-duckos-x\n.network", "anything.network", "10-duckos-x.service"} {
		if d.Replace(nil, map[string]string{bad: "x"}) == nil {
			t.Fatal("unsafe filename", bad)
		}
	}
}
func TestArtifactInterruptedWriteRecovery(t *testing.T) {
	d, path := artifactTest(t)
	name := "10-duckos-inside.network"
	old := map[string]string{name: "previous configuration"}
	next := map[string]string{name: "candidate configuration"}
	if err := d.Replace(nil, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, name+".duckos-tmp"), []byte("candidate con"), 0600); err != nil {
		t.Fatal(err)
	}
	if d.Check(old, next) == nil {
		t.Fatal("staging residue ignored")
	}
	if err := d.Restore(old, next); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(path, name))
	if string(got) != old[name] {
		t.Fatal("recovery did not restore before")
	}
	if _, err := os.Stat(filepath.Join(path, name+".duckos-tmp")); !os.IsNotExist(err) {
		t.Fatal("staging not reclaimed")
	}
}
func TestArtifactSymlinkNeverFollowed(t *testing.T) {
	d, path := artifactTest(t)
	name := "10-duckos-inside.network"
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(target, []byte("preserve"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(path, name)); err != nil {
		t.Fatal(err)
	}
	if d.Restore(map[string]string{name: "preserve"}, map[string]string{name: "candidate"}) == nil {
		t.Fatal("symlink followed")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "preserve" {
		t.Fatal("external target changed")
	}
}
