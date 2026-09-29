package devices

import (
	"fmt"
	"testing"
	"time"
)

func obs(mac, address, link string, at time.Time) Observation {
	return Observation{At: at, MAC: mac, Address: address, Link: link, Source: "test"}
}

func TestDeviceValidateRejectsInvalidInputs(t *testing.T) {
	d := Device{ID: "short", FirstSeen: time.Now().UTC()}
	if err := d.Validate(); err == nil {
		t.Fatal("invalid id was accepted")
	}
	d.ID = "0123456789abcdef0123456789abcdef"
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	d.MAC = "01:02:03:04:05"
	if err := d.Validate(); err == nil {
		t.Fatal("invalid MAC was accepted")
	}
}

func TestCatalogRecordIsIdempotentByMAC(t *testing.T) {
	c := NewCatalog()
	at := time.Now().UTC()
	d1, err := c.Record(obs("01:02:03:04:05:06", "192.0.2.10", "testlan0", at))
	if err != nil {
		t.Fatal(err)
	}
	d2, err := c.Record(obs("01:02:03:04:05:06", "192.0.2.10", "testlan0", at.Add(time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	if d1.ID != d2.ID {
		t.Fatal("idempotent record produced different ids")
	}
	if d2.LastSeen.Equal(d1.LastSeen) {
		t.Fatal("LastSeen did not advance")
	}
}

func TestCatalogRecordMergesMultipleLinks(t *testing.T) {
	c := NewCatalog()
	at := time.Now().UTC()
	if _, err := c.Record(obs("01:02:03:04:05:06", "192.0.2.10", "testlan0", at)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Record(obs("01:02:03:04:05:06", "192.0.2.10", "testguest0", at.Add(time.Second))); err != nil {
		t.Fatal(err)
	}
	list := c.ListDevices()
	if len(list) != 1 {
		t.Fatal("expected one device, got", len(list))
	}
	if len(list[0].Links) != 2 {
		t.Fatal("expected two links, got", list[0].Links)
	}
}

func TestCatalogRecordDerivesIDFromAddress(t *testing.T) {
	c := NewCatalog()
	at := time.Now().UTC()
	d, err := c.Record(Observation{At: at, Address: "2001:db8::1", Link: "testlan0", Source: "ndp"})
	if err != nil {
		t.Fatal(err)
	}
	if d.MAC != "" {
		t.Fatal("device unexpectedly carried a MAC")
	}
	if d.ID == "" {
		t.Fatal("id was not derived from the address")
	}
}

func TestCatalogRespectsCapacity(t *testing.T) {
	c := NewCatalog()
	at := time.Now().UTC()
	// Fill the catalog to capacity. The unique MAC plus unique
	// address pair guarantees every record creates a new device.
	firstMAC := ""
	for i := 0; i < MaxDevices; i++ {
		if _, err := c.Record(Observation{At: at, MAC: uniqueMAC(i), Address: intToAddr(i), Link: "testlan0", Source: "test"}); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstMAC = uniqueMAC(0)
		}
	}
	// Adding a new observation that matches an existing MAC
	// updates the existing record rather than creating a new one,
	// so it must not be rejected for capacity.
	if _, err := c.Record(Observation{At: at, MAC: firstMAC, Address: intToAddr(0), Link: "testlan0", Source: "test"}); err != nil {
		t.Fatal("update of existing device was rejected", err)
	}
	// A genuinely new observation must be rejected for capacity.
	_, err := c.Record(Observation{At: at, MAC: "ff:ff:ff:ff:ff:ff", Address: "203.0.113.1", Link: "testlan0", Source: "test"})
	if err == nil {
		t.Fatal("capacity was not enforced")
	}
}

func uniqueMAC(i int) string {
	// Encode the int into a 6-byte MAC. A real host would
	// assign the OUI; the catalog only needs a canonical form.
	b := []byte{0x02, 0x00, 0x00, byte(i >> 16), byte(i >> 8), byte(i)}
	return macBytesToString(b)
}

func macBytesToString(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 0, 17)
	for i, c := range b {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, hex[c>>4], hex[c&0x0F])
	}
	return string(out)
}

func TestCatalogForgetsDevice(t *testing.T) {
	c := NewCatalog()
	at := time.Now().UTC()
	d, _ := c.Record(obs("01:02:03:04:05:06", "192.0.2.10", "testlan0", at))
	if err := c.ForgetDevice(d.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.GetDevice(d.ID); ok {
		t.Fatal("device was not forgotten")
	}
}

func TestPolicyValidate(t *testing.T) {
	p := Policy{ID: "0123456789abcdef0123456789abcdef", Link: "testlan0", Role: RoleGuest, UpdatedAt: time.Now().UTC()}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	p.Role = "unknown"
	if err := p.Validate(); err == nil {
		t.Fatal("unknown role was accepted")
	}
	p.Role = RoleQuarantined
	p.Link = "Bad Link"
	if err := p.Validate(); err == nil {
		t.Fatal("invalid link was accepted")
	}
}

func TestPolicyIsQuarantinedGates(t *testing.T) {
	c := NewCatalog()
	at := time.Now().UTC()
	d, _ := c.Record(obs("01:02:03:04:05:06", "192.0.2.10", "testlan0", at))
	if c.IsQuarantined(d.ID, "testlan0") {
		t.Fatal("default policy must not be quarantined")
	}
	if err := c.UpsertPolicy(Policy{ID: d.ID, Link: "testlan0", Role: RoleQuarantined, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if !c.IsQuarantined(d.ID, "testlan0") {
		t.Fatal("device was not quarantined after policy")
	}
}

func TestPolicyIsPerLink(t *testing.T) {
	c := NewCatalog()
	at := time.Now().UTC()
	d, _ := c.Record(obs("01:02:03:04:05:06", "192.0.2.10", "testlan0", at))
	_, _ = c.Record(obs("01:02:03:04:05:06", "192.0.2.10", "testguest0", at))
	if err := c.UpsertPolicy(Policy{ID: d.ID, Link: "testlan0", Role: RoleQuarantined, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if !c.IsQuarantined(d.ID, "testlan0") {
		t.Fatal("expected quarantine on testlan0")
	}
	if c.IsQuarantined(d.ID, "testguest0") {
		t.Fatal("quarantine leaked to testguest0")
	}
}

func TestForgetDeviceRefusesWhenPolicyActive(t *testing.T) {
	c := NewCatalog()
	at := time.Now().UTC()
	d, _ := c.Record(obs("01:02:03:04:05:06", "192.0.2.10", "testlan0", at))
	_ = c.UpsertPolicy(Policy{ID: d.ID, Link: "testlan0", Role: RoleGuest, UpdatedAt: time.Now().UTC()})
	if err := c.ForgetDevice(d.ID); err == nil {
		t.Fatal("device with active policy was forgotten")
	}
}

func TestRemovePolicy(t *testing.T) {
	c := NewCatalog()
	at := time.Now().UTC()
	d, _ := c.Record(obs("01:02:03:04:05:06", "192.0.2.10", "testlan0", at))
	_ = c.UpsertPolicy(Policy{ID: d.ID, Link: "testlan0", Role: RoleIoT, UpdatedAt: time.Now().UTC()})
	if err := c.RemovePolicy(d.ID, "testlan0"); err != nil {
		t.Fatal(err)
	}
	if c.IsQuarantined(d.ID, "testlan0") {
		t.Fatal("policy was not removed")
	}
}

func TestEffectiveRoleDefaultsToTrusted(t *testing.T) {
	c := NewCatalog()
	at := time.Now().UTC()
	d, _ := c.Record(obs("01:02:03:04:05:06", "192.0.2.10", "testlan0", at))
	if got := c.EffectiveRole(d.ID, "testlan0"); got != RoleTrusted {
		t.Fatal("default role was not trusted", got)
	}
}

// intToAddr encodes i into a unique 10.0.x.y address. The function
// covers the full 10.0.0.0/20 block, which is exactly 4096 addresses
// — enough to fill the catalog at capacity and prove that the next
// distinct observation is rejected.
func intToAddr(i int) string {
	return fmt.Sprintf("10.0.%d.%d", (i>>8)&0xff, i&0xff)
}
