package network

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDNSMasqPreflightUsesOnlyRenderedCandidate(t *testing.T) {
	calls := 0
	run := func(_ context.Context, raw []byte) error {
		calls++
		if !strings.Contains(string(raw), "no-resolv\nno-hosts\n") || !strings.Contains(string(raw), "interface=testlan0\n") || strings.Contains(string(raw), "conf-file=") || strings.Contains(string(raw), "conf-script=") {
			t.Fatal("unsafe DNS input boundary")
		}
		return nil
	}
	if err := checkDNSMasq(context.Background(), fixture(), run); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("candidate was not checked")
	}
	if err := checkDNSMasq(context.Background(), Config{Version: 1}, run); err != nil || calls != 1 {
		t.Fatal("empty intent invoked host defaults")
	}
	invalid := fixture()
	invalid.DNS.Records[0].Name = "x\nconf-script=/tmp/untrusted"
	if err := checkDNSMasq(context.Background(), invalid, run); err == nil || calls != 1 {
		t.Fatal("invalid intent reached dnsmasq")
	}
	if err := checkDNSMasq(context.Background(), fixture(), func(context.Context, []byte) error { return errors.New("synthetic syntax error") }); err == nil {
		t.Fatal("syntax failure ignored")
	}
}
