package network

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHostSnapshotBindingValidation(t *testing.T) {
	c := fixture()
	s := HostSnapshot{Version: 1, CapturedAt: time.Now(), Links: []HostLink{
		{Name: "testwan0", Index: 2, MAC: "02:00:00:00:00:01", MTU: 1500, Up: true, Addresses: []string{"192.0.2.2/24"}},
		{Name: "testlan0", Index: 3, MAC: "02:00:00:00:00:02", MTU: 1500, Addresses: []string{"fe80::1/64"}},
	}}
	raw, _ := json.Marshal(s)
	if _, err := DecodeHostSnapshot(raw, c, c); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*HostSnapshot){
		func(s *HostSnapshot) { s.Links[0].Name = "../../unsafe" },
		func(s *HostSnapshot) { s.Links[0].Name = "unbound0" },
		func(s *HostSnapshot) { s.Links[0].Index = 3 },
		func(s *HostSnapshot) { s.Links[0].Addresses = []string{"not-an-address"} },
		func(s *HostSnapshot) { s.Links = s.Links[:1] },
	} {
		var candidate HostSnapshot
		_ = json.Unmarshal(raw, &candidate)
		mutate(&candidate)
		bad, _ := json.Marshal(candidate)
		if _, err := DecodeHostSnapshot(bad, c, c); err == nil {
			t.Fatal("unsafe snapshot accepted")
		}
	}
}
