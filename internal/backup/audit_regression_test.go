package backup

import "testing"

func TestAuditRemapDoesNotMutateBackup(t *testing.T) {
	in := LogicalManifest{Interfaces: []InterfaceBinding{{Logical: "wan", Physical: "eth0"}}}
	out, missing := Remap(in, []RemapRequest{{Logical: "wan", Physical: "eth1"}})
	if len(missing) != 0 || out.Interfaces[0].Physical != "eth1" {
		t.Fatal("valid remap failed")
	}
	if in.Interfaces[0].Physical != "eth0" {
		t.Fatal("preview modified source backup")
	}
}
