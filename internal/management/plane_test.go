// management/plane_test.go pins the V1
// management-plane contract.
package management

import "testing"

func TestPathValid(t *testing.T) {
	good := Path{Kind: PathInterface, Target: "eth0", Roles: []AccessRole{RoleWeb, RoleAPI}}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := Path{Kind: "garbage", Target: "eth0", Roles: []AccessRole{RoleWeb}}
	if err := bad.Valid(); err == nil {
		t.Fatal("invalid kind must be rejected")
	}
	badRole := Path{Kind: PathInterface, Target: "eth0", Roles: []AccessRole{"superuser"}}
	if err := badRole.Valid(); err == nil {
		t.Fatal("invalid role must be rejected")
	}
}

func TestManagerAddAndAllows(t *testing.T) {
	m := NewManager(Policy{})
	if err := m.AddPath(Path{Kind: PathInterface, Target: "eth0", Roles: []AccessRole{RoleAll}}); err != nil {
		t.Fatal(err)
	}
	if !m.Allows(PathInterface, "eth0", "api") {
		t.Fatal("RoleAll must permit any role")
	}
	if m.Allows(PathInterface, "eth1", "api") {
		t.Fatal("non-registered path must be refused")
	}
}

func TestManagerOOBAlwaysAllows(t *testing.T) {
	m := NewManager(Policy{OOBInterface: "eth1"})
	if !m.Allows(PathInterface, "eth1", "ssh") {
		t.Fatal("OOB interface must be allowed for any role")
	}
}

func TestManagerRemovePath(t *testing.T) {
	m := NewManager(Policy{})
	if err := m.AddPath(Path{Kind: PathVLAN, Target: "vlan-100", Roles: []AccessRole{RoleAPI}}); err != nil {
		t.Fatal(err)
	}
	if err := m.RemovePath(PathVLAN, "vlan-100"); err != nil {
		t.Fatal(err)
	}
	if m.Allows(PathVLAN, "vlan-100", "api") {
		t.Fatal("after Remove, Allows must be false")
	}
	if err := m.RemovePath(PathVLAN, "vlan-100"); err == nil {
		t.Fatal("second Remove must fail")
	}
}

func TestManagerDuplicatePathRefused(t *testing.T) {
	m := NewManager(Policy{})
	if err := m.AddPath(Path{Kind: PathTunnel, Target: "wg-mgmt", Roles: []AccessRole{RoleAll}}); err != nil {
		t.Fatal(err)
	}
	if err := m.AddPath(Path{Kind: PathTunnel, Target: "wg-mgmt", Roles: []AccessRole{RoleWeb}}); err == nil {
		t.Fatal("duplicate path must be rejected")
	}
}

func TestManagerSetRejectsInvalidPaths(t *testing.T) {
	m := NewManager(Policy{})
	err := m.Set(Policy{Paths: []Path{{Kind: "garbage", Target: "x", Roles: []AccessRole{RoleWeb}}}})
	if err == nil {
		t.Fatal("Set with invalid path must be rejected")
	}
}
