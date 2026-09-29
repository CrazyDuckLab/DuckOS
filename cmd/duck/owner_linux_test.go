package main

import (
	"os"
	"testing"
)

func TestOwnerRecoveryCLIGate(t *testing.T) {
	if os.Geteuid() != 0 {
		if err := dispatch([]string{"owner", "reset"}); err == nil || err.Error() != "Owner recovery requires effective root" {
			t.Fatal(err)
		}
	}
	if err := dispatch([]string{"owner", "reset", "--identity-dir=/tmp/other"}); err == nil {
		t.Fatal("path override accepted")
	}
}
