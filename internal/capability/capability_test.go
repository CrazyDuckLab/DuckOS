// capability/capability_test.go pins the
// V1 hardware-capability surface.
package capability

import "testing"

func TestStatusValid(t *testing.T) {
	for _, s := range []Status{StatusSupported, StatusUnsupported, StatusDisabled, StatusUnknown} {
		if !s.Valid() {
			t.Fatalf("%s must be valid", s)
		}
	}
	if Status("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
}

func TestManagerRegisterProbe(t *testing.T) {
	m := NewManager()
	if err := m.RegisterProbe(FeatureWiFi, func() Report {
		return Report{Status: StatusSupported, Detail: "ath10k"}
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterProbe(FeaturePoE, nil); err == nil {
		t.Fatal("nil probe must fail")
	}
}

func TestManagerDetect(t *testing.T) {
	m := NewManager()
	m.RegisterProbe(FeatureWiFi, func() Report { return Report{Status: StatusSupported, Detail: "ath10k"} })
	m.RegisterProbe(FeaturePoE, func() Report { return Report{Status: StatusUnsupported, Detail: "no PoE hardware"} })
	m.SetReport(Report{Feature: FeatureStorage, Status: StatusSupported, Detail: "nvme"})
	reports := m.Detect()
	statuses := map[Feature]Status{}
	for _, r := range reports {
		statuses[r.Feature] = r.Status
	}
	if statuses[FeatureWiFi] != StatusSupported {
		t.Fatalf("wifi: %s", statuses[FeatureWiFi])
	}
	if statuses[FeaturePoE] != StatusUnsupported {
		t.Fatalf("poe: %s", statuses[FeaturePoE])
	}
	if statuses[FeatureStorage] != StatusSupported {
		t.Fatalf("storage: %s", statuses[FeatureStorage])
	}
}

func TestManagerDetectProbePanic(t *testing.T) {
	m := NewManager()
	m.RegisterProbe(FeatureWiFi, func() Report { panic("boom") })
	reports := m.Detect()
	if len(reports) != 1 || reports[0].Status != StatusUnknown {
		t.Fatalf("expected unknown, got %+v", reports)
	}
}

func TestManagerCheckAction(t *testing.T) {
	m := NewManager()
	m.SetReport(Report{Feature: FeatureWiFi, Status: StatusSupported})
	m.SetReport(Report{Feature: FeaturePoE, Status: StatusDisabled})
	m.SetReport(Report{Feature: FeatureSFP, Status: StatusUnsupported})
	res := m.CheckAction([]Feature{FeatureWiFi, FeaturePoE, FeatureSFP})
	if res.Supported {
		t.Fatal("expected not supported")
	}
	if len(res.Missing) != 1 || res.Missing[0] != FeatureSFP {
		t.Fatalf("missing: %v", res.Missing)
	}
	if len(res.Disabled) != 1 || res.Disabled[0] != FeaturePoE {
		t.Fatalf("disabled: %v", res.Disabled)
	}
	if res.Reason == "" {
		t.Fatal("reason must be set")
	}
}

func TestManagerCheckActionAllSupported(t *testing.T) {
	m := NewManager()
	m.SetReport(Report{Feature: FeatureWiFi, Status: StatusSupported})
	m.SetReport(Report{Feature: FeaturePoE, Status: StatusSupported})
	res := m.CheckAction([]Feature{FeatureWiFi, FeaturePoE})
	if !res.Supported {
		t.Fatal("expected supported")
	}
}

func TestManagerCheckActionUnknown(t *testing.T) {
	m := NewManager()
	res := m.CheckAction([]Feature{FeatureWiFi})
	if res.Supported {
		t.Fatal("unknown must be missing")
	}
}
