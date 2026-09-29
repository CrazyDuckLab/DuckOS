package network

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"runtime"
	"time"
)

// CheckDNSMasq validates generated candidate syntax without starting a daemon,
// binding sockets or reading the host's default dnsmasq configuration. It is a
// preflight gate, not DNS availability or DHCP lease-delivery verification.
func CheckDNSMasq(ctx context.Context, candidate Config) error {
	return checkDNSMasq(ctx, candidate, runDNSMasqCheck)
}

func checkDNSMasq(ctx context.Context, candidate Config, run func(context.Context, []byte) error) error {
	plan, err := Render(candidate)
	if err != nil {
		return err
	}
	if plan.DNSMasq == "" {
		return nil
	} // Empty intent requires service stop, not startup with defaults.
	if len(plan.DNSMasq) > 256<<10 {
		return errors.New("generated DNS configuration exceeds limit")
	}
	return run(ctx, []byte(plan.DNSMasq))
}

func runDNSMasqCheck(parent context.Context, candidate []byte) error {
	if runtime.GOOS != "linux" || len(candidate) == 0 || len(candidate) > 256<<10 {
		return errors.New("dnsmasq syntax check unavailable")
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/sbin/dnsmasq", "--test", "--conf-file=-")
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin", "LANG=C", "LC_ALL=C"}
	cmd.Dir = "/"
	cmd.Stdin = bytes.NewReader(candidate)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return errors.New("dnsmasq rejected candidate or syntax check exceeded limits")
	}
	return nil
}
