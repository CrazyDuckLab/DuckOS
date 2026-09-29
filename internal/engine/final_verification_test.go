package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

func TestFinalVerificationReceiptFailsBeforeWork(t *testing.T) {
	c := freshConfig(t)
	if err := c.EnableNetworking(&Networking{}); err != nil {
		t.Fatal(err)
	}
	if err := c.Reconcile(context.Background(), state.Outcome{}); !errors.Is(err, ErrConfigRuntimeUnavailable) {
		t.Fatal("caller receipt accepted")
	}
}

func TestFinalVerificationNoInventedOutcome(t *testing.T) {
	for _, st := range []NetworkState{
		{},
		{Pending: &state.Transaction{ID: "tx"}},
		{Outcomes: []state.Outcome{{TransactionID: "tx", Result: "rolled-back"}}},
	} {
		if got := outcomeFromState(st, "tx", "committed"); got.Receipt != nil {
			t.Fatal("commit receipt fabricated without a reconciled matching outcome")
		}
	}
}
