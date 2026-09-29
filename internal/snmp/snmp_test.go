// snmp/snmp_test.go pins the V1 SNMPv3 model.
package snmp

import "testing"

func TestVersionValid(t *testing.T) {
	if !VersionV3.Valid() || !VersionV2c.Valid() {
		t.Fatal("known versions must be valid")
	}
	if Version("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
}

func TestUserValid(t *testing.T) {
	good := User{Username: "monitor", AuthProto: AuthSHA256, AuthVault: "vault-1", PrivProto: PrivAES256, PrivVault: "vault-2"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := User{Username: "monitor", AuthProto: AuthSHA256, AuthVault: "v", PrivProto: PrivAES256}
	if err := bad.Valid(); err == nil {
		t.Fatal("priv without vault must fail")
	}
	bad2 := User{Username: "monitor", AuthProto: "garbage", AuthVault: "v", PrivProto: PrivAES256, PrivVault: "v"}
	if err := bad2.Valid(); err == nil {
		t.Fatal("unknown auth must fail")
	}
}

func TestTrapValid(t *testing.T) {
	v3 := Trap{Host: "10.0.0.1", Port: 162, Version: VersionV3, Inform: true}
	if err := v3.Valid(); err != nil {
		t.Fatal(err)
	}
	v2c := Trap{Host: "10.0.0.1", Port: 162, Version: VersionV2c, Community: "public"}
	if err := v2c.Valid(); err != nil {
		t.Fatal(err)
	}
	v2cNoCom := Trap{Host: "10.0.0.1", Port: 162, Version: VersionV2c}
	if err := v2cNoCom.Valid(); err == nil {
		t.Fatal("v2c without community must fail")
	}
}

func TestManagerCRUD(t *testing.T) {
	m := NewManager()
	if err := m.SetUser(User{Username: "monitor", AuthProto: AuthSHA256, AuthVault: "v", PrivProto: PrivAES256, PrivVault: "v"}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTrap(Trap{Host: "10.0.0.1", Port: 162, Version: VersionV3}); err != nil {
		t.Fatal(err)
	}
	if len(m.Users()) != 1 || len(m.Traps()) != 1 {
		t.Fatalf("counts: %d %d", len(m.Users()), len(m.Traps()))
	}
}
