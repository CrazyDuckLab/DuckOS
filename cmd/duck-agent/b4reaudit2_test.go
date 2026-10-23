// Package main / b4reaudit2_test.go: node-auth limiter regressions
// for the second independent re-audit (2026-10-23).
package main

import (
	"fmt"
	"testing"
)

// The limiter key must be the stable source host: rotating ephemeral
// source ports must not mint a fresh attempt budget.
func TestB403_LimiterKeyIgnoresSourcePort(t *testing.T) {
	limit := nodeAuthLimiter()
	for i := 0; i < 61; i++ {
		if !limit(fmt.Sprintf("192.0.2.1:%d", 10000+i)) {
			return
		}
	}
	t.Fatal("same source IP exceeds 60 attempts by changing source port")
}

// Per-key limit and hard bound on tracked peers (memory bound).
func TestB403_LimiterBounds(t *testing.T) {
	limit := nodeAuthLimiter()
	for i := 0; i < 60; i++ {
		if !limit("192.0.2.1:12345") {
			t.Fatal("early limit")
		}
	}
	if limit("192.0.2.1:12345") {
		t.Fatal("no per-key limit")
	}
	limit = nodeAuthLimiter()
	for i := 0; i < 4096; i++ {
		if !limit(fmt.Sprint(i)) {
			t.Fatal("early peer capacity")
		}
	}
	if limit("extra-peer") {
		t.Fatal("unbounded peer map")
	}
}
