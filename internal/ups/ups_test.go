// ups_test.go covers the model, the classifier
// and the textual runner. The Agent and Linux
// integration tests live in their own files.
package ups

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sampleDevice() Device {
	return Device{ID: "ups-1", Name: "Office UPS", Driver: "usbhid-ups", Port: "auto", Description: "primary"}
}

func TestDeviceValid(t *testing.T) {
	d := sampleDevice()
	if err := d.Valid(); err != nil {
		t.Fatalf("expected base device to validate, got %v", err)
	}
	cases := []struct {
		name    string
		mut     func(*Device)
		wantErr string
	}{
		{"bad id", func(d *Device) { d.ID = "9" }, "id is not canonical"},
		{"bad name", func(d *Device) { d.Name = "9bad" }, "name is not canonical"},
		{"bad driver", func(d *Device) { d.Driver = "9bad" }, "driver is not canonical"},
		{"empty port", func(d *Device) { d.Port = "" }, "device port is empty"},
		{"oversized port", func(d *Device) { d.Port = strings.Repeat("a", MaxPortLen+1) }, "port is oversized"},
		{"shell metachar", func(d *Device) { d.Port = "auto; rm -rf /" }, "shell metacharacter"},
		{"description oversized", func(d *Device) { d.Description = strings.Repeat("a", MaxDescription+1) }, "description is oversized"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := sampleDevice()
			tc.mut(&d)
			err := d.Valid()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestTopologyValid(t *testing.T) {
	d1 := sampleDevice()
	d2 := sampleDevice()
	d2.ID = "ups-2"
	d2.Port = "/dev/ttyS0"
	if _, err := Build([]Device{d1, d2}, time.Unix(1, 0)); err != nil {
		t.Fatalf("expected build to succeed, got %v", err)
	}
	if _, err := Build(nil, time.Now()); err == nil {
		t.Fatalf("expected build to fail with no devices")
	}
	if _, err := Build([]Device{sampleDevice(), sampleDevice()}, time.Now()); err == nil {
		t.Fatalf("expected build to fail on duplicate id")
	}
	wrong := Topology{SchemaVersion: "0", Devices: []Device{sampleDevice()}}
	if err := wrong.Valid(); err == nil || !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("expected schema version error, got %v", err)
	}
}

func TestBuildSorts(t *testing.T) {
	d1 := sampleDevice()
	d2 := sampleDevice()
	d2.ID = "ups-2"
	tp, err := Build([]Device{d2, d1}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if tp.Devices[0].ID != "ups-1" || tp.Devices[1].ID != "ups-2" {
		t.Fatalf("expected sorted devices, got %+v", tp.Devices)
	}
	if tp.GeneratedAt != 1 {
		t.Fatalf("expected generated=1, got %d", tp.GeneratedAt)
	}
}

func TestEncodeDecodeRoundtrip(t *testing.T) {
	tp, err := Build([]Device{sampleDevice()}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	data, err := EncodeTopology(tp)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	parsed, err := ParseTopology(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	da, _ := json.Marshal(tp)
	db, _ := json.Marshal(parsed)
	if !bytes.Equal(da, db) {
		t.Fatalf("roundtrip mismatch")
	}
	// unknown field
	bad := []byte(`{"schemaVersion":"1","devices":[{"id":"ups-1","name":"Office UPS","driver":"usbhid-ups","port":"auto","x":1}]}`)
	if _, err := ParseTopology(bad); err == nil {
		t.Fatalf("expected parse to fail on unknown field")
	}
	// truncated
	if _, err := ParseTopology(data[:5]); err == nil {
		t.Fatalf("expected parse to fail on truncated")
	}
}

func TestVariableValid(t *testing.T) {
	good := Variable{Key: "battery.charge", Value: "85"}
	if err := good.Valid(); err != nil {
		t.Fatalf("expected good var to validate: %v", err)
	}
	cases := []struct {
		name    string
		v       Variable
		wantErr string
	}{
		{"empty key", Variable{Value: "x"}, "key is not canonical"},
		{"empty value", Variable{Key: "battery.charge"}, "value is empty"},
		{"oversized value", Variable{Key: "battery.charge", Value: strings.Repeat("a", MaxVariableValue+1)}, "value is oversized"},
		{"newline in value", Variable{Key: "battery.charge", Value: "a\nb"}, "value contains newline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.v.Valid()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestClassifyHealth(t *testing.T) {
	cases := []struct {
		status string
		charge int
		want   Health
	}{
		{"OL", 100, HealthOnline},
		{"OL CHRG", 20, HealthDegraded},
		{"OB DISCHRG", 0, HealthOnBattery},
		{"LB", 0, HealthLowBattery},
		{"RB", 0, HealthReplaceBatt},
		{"CHRG", 0, HealthCharging},
		{"DISCHRG", 0, HealthDischarging},
		{"", 0, HealthUnknown},
		{"weird", 0, HealthUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			if got := ClassifyHealth(tc.status, tc.charge); got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}
}

func TestIsCritical(t *testing.T) {
	for h, want := range map[Health]bool{
		HealthOnline:      false,
		HealthOnBattery:   false,
		HealthLowBattery:  true,
		HealthReplaceBatt: true,
		HealthLost:        true,
		HealthDegraded:    true,
		HealthUnknown:     false,
	} {
		if got := IsCritical(h); got != want {
			t.Fatalf("IsCritical(%s) = %v, want %v", h, got, want)
		}
	}
}

func TestStateRecordProbe(t *testing.T) {
	tp, _ := Build([]Device{sampleDevice()}, time.Unix(1, 0))
	st := NewState(tp)
	if st.HealthOf("ups-1") != HealthUnconfigured {
		t.Fatalf("expected initial health=unconfigured")
	}
	vars := []Variable{{Key: "battery.charge", Value: "85"}, {Key: "ups.status", Value: "OL"}}
	st.RecordProbe("ups-1", HealthOnline, vars, time.Unix(7, 0))
	if st.HealthOf("ups-1") != HealthOnline {
		t.Fatalf("expected health=online")
	}
	if st.Status().ProbedAt != 7 {
		t.Fatalf("expected probedAt=7, got %d", st.Status().ProbedAt)
	}
	// invalid health falls back to unknown
	st.RecordProbe("ups-1", Health("bogus"), vars, time.Now())
	if st.HealthOf("ups-1") != HealthUnknown {
		t.Fatalf("expected unknown for bogus health")
	}
	// unknown id is ignored
	st.RecordProbe("missing", HealthOnline, vars, time.Now())
	// bad variable is dropped
	st.RecordProbe("ups-1", HealthOnline, []Variable{{Key: "9bad", Value: "x"}, {Key: "battery.charge", Value: "85"}}, time.Now())
	if len(st.Status().Variables["ups-1"]) != 1 {
		t.Fatalf("expected bad variable to be dropped, got %+v", st.Status().Variables["ups-1"])
	}
	st.MarkError(strings.Repeat("x", 1024))
	if len(st.Status().LastError) > 256 {
		t.Fatalf("expected error truncated, got %d", len(st.Status().LastError))
	}
}

func TestStateUpdateTopology(t *testing.T) {
	tp, _ := Build([]Device{sampleDevice()}, time.Unix(1, 0))
	st := NewState(tp)
	st.RecordProbe("ups-1", HealthOnline, []Variable{{Key: "battery.charge", Value: "100"}}, time.Now())
	// build a new topology with a new device and
	// drop the existing one
	d2 := sampleDevice()
	d2.ID = "ups-2"
	d2.Port = "/dev/ttyS0"
	tp2, _ := Build([]Device{d2}, time.Unix(2, 0))
	if err := st.UpdateTopology(tp2); err != nil {
		t.Fatalf("update: %v", err)
	}
	if st.HealthOf("ups-1") != HealthUnknown {
		t.Fatalf("expected removed device to be evicted")
	}
	if st.HealthOf("ups-2") != HealthUnconfigured {
		t.Fatalf("expected new device to start as unconfigured")
	}
	// bad update is rejected
	if err := st.UpdateTopology(Topology{SchemaVersion: "0"}); err == nil {
		t.Fatalf("expected bad update to be rejected")
	}
}

func TestSummarise(t *testing.T) {
	tp, _ := Build([]Device{sampleDevice()}, time.Unix(1, 0))
	st := NewState(tp)
	st.RecordProbe("ups-1", HealthLowBattery, nil, time.Now())
	s := st.Status()
	sum := Summarise(s)
	if len(sum) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(sum))
	}
	if sum[0].Health != HealthLowBattery {
		t.Fatalf("expected health=low-battery, got %s", sum[0].Health)
	}
	if !sum[0].Critical {
		t.Fatalf("expected critical=true for low-battery")
	}
}

func TestParseUpscOutput(t *testing.T) {
	in := "battery.charge: 85\nbattery.runtime: 1800\nups.status: OL\ninput.voltage: 230.0\nbroken line\n:novalue\n"
	vars := parseUpscOutput([]byte(in))
	if len(vars) != 4 {
		t.Fatalf("expected 4 vars, got %d: %+v", len(vars), vars)
	}
	if vars[0].Key != "battery.charge" || vars[0].Value != "85" {
		t.Fatalf("expected first var to be battery.charge=85, got %+v", vars[0])
	}
	// max variables cap
	big := strings.Repeat("k: v\n", MaxVariables*2)
	out := parseUpscOutput([]byte(big))
	if len(out) != MaxVariables {
		t.Fatalf("expected at most %d, got %d", MaxVariables, len(out))
	}
}

func TestFirstIntVar(t *testing.T) {
	vars := []Variable{{Key: "battery.charge", Value: "85"}, {Key: "battery.charge.low", Value: "20"}}
	if got := firstIntVar(vars, "battery.charge"); got != 85 {
		t.Fatalf("expected 85, got %d", got)
	}
	if got := firstIntVar(vars, "missing"); got != 0 {
		t.Fatalf("expected 0 for missing, got %d", got)
	}
	vars2 := []Variable{{Key: "battery.charge", Value: "85%"}}
	if got := firstIntVar(vars2, "battery.charge"); got != 85 {
		t.Fatalf("expected 85 with %% suffix, got %d", got)
	}
	vars3 := []Variable{{Key: "battery.charge", Value: "abc"}}
	if got := firstIntVar(vars3, "battery.charge"); got != 0 {
		t.Fatalf("expected 0 for non-numeric, got %d", got)
	}
}

// fakeRunner records every call. The tests
// inject the fake through the Runner.
type fakeRunner struct {
	calls   int32
	failOn  int
	results []string
	errs    []error
}

func (f *fakeRunner) Run(_ context.Context, _ []string) ([]byte, error) {
	idx := int(atomic.AddInt32(&f.calls, 1)) - 1
	if f.failOn > 0 && idx+1 == f.failOn {
		if idx < len(f.errs) {
			return nil, f.errs[idx]
		}
		return nil, errors.New("synthetic")
	}
	if idx < len(f.results) {
		return []byte(f.results[idx]), nil
	}
	return nil, nil
}

func TestRunnerProbe(t *testing.T) {
	r := &Runner{Exec: &fakeRunner{results: []string{"ups.status: OL\nbattery.charge: 85\n"}}, Clock: func() time.Time { return time.Unix(0, 0) }, Timeout: time.Second}
	res := r.Probe(context.Background(), sampleDevice())
	if res.Err != nil {
		t.Fatalf("expected no error, got %v", res.Err)
	}
	if res.Health != HealthOnline {
		t.Fatalf("expected health=online, got %s", res.Health)
	}
	if len(res.Variables) != 2 {
		t.Fatalf("expected 2 vars, got %d", len(res.Variables))
	}
}

func TestRunnerProbeInvalidDevice(t *testing.T) {
	r := &Runner{Exec: &fakeRunner{}, Clock: func() time.Time { return time.Unix(0, 0) }, Timeout: time.Second}
	bad := sampleDevice()
	bad.ID = "9bad"
	res := r.Probe(context.Background(), bad)
	if res.Err == nil {
		t.Fatalf("expected error for invalid device")
	}
	if res.Health != HealthUnknown {
		t.Fatalf("expected health=unknown, got %s", res.Health)
	}
}

func TestRunnerProbeExecutorFailure(t *testing.T) {
	r := &Runner{Exec: &fakeRunner{failOn: 1, errs: []error{errors.New("daemon down")}}, Clock: func() time.Time { return time.Unix(0, 0) }, Timeout: time.Second}
	res := r.Probe(context.Background(), sampleDevice())
	if res.Err == nil {
		t.Fatalf("expected error from executor")
	}
	if res.Health != HealthUnknown {
		t.Fatalf("expected health=unknown, got %s", res.Health)
	}
}

func TestRunnerProbeAll(t *testing.T) {
	d2 := sampleDevice()
	d2.ID = "ups-2"
	d2.Port = "/dev/ttyS0"
	r := &Runner{Exec: &fakeRunner{results: []string{"ups.status: OL\n", "ups.status: OB DISCHRG\n"}}, Clock: func() time.Time { return time.Unix(0, 0) }, Timeout: time.Second}
	res := r.ProbeAll(context.Background(), []Device{sampleDevice(), d2})
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	if res[0].Health != HealthOnline || res[1].Health != HealthOnBattery {
		t.Fatalf("unexpected healths: %s, %s", res[0].Health, res[1].Health)
	}
}

func TestNewExecRunnerDefaultsToUpsc(t *testing.T) {
	e := NewExecRunner("")
	if e.Binary == "" {
		t.Fatalf("expected default binary path")
	}
	if !strings.Contains(e.Binary, "upsc") {
		t.Fatalf("expected default to mention upsc, got %q", e.Binary)
	}
}

func TestRunnerRejectsEmptyArgv(t *testing.T) {
	e := NewExecRunner("/bin/true")
	if _, err := e.Run(context.Background(), nil); err == nil {
		t.Fatalf("expected empty argv to be rejected")
	}
}
