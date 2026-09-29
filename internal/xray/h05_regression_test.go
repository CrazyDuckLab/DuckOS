package xray

import (
	"context"
	"errors"
	"testing"
)

func TestH05LastUserRemovalMustApplyRuntime(t *testing.T) {
	for _, fail := range []bool{false, true} {
		cat, err := OpenFileCatalog(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		s, u := validServerFixture(), validUserFixture(validServerFixture().ID)
		if err := cat.UpsertServer(context.Background(), s); err != nil {
			t.Fatal(err)
		}
		if err := cat.UpsertUser(context.Background(), u); err != nil {
			t.Fatal(err)
		}
		calls := 0
		a := API{Catalog: cat, Apply: func(_ context.Context, cfg Config) error {
			calls++
			if len(cfg.Inbounds) != 0 {
				t.Fatal("revoked listener remains")
			}
			if fail {
				return errors.New("runtime refusal")
			}
			return nil
		}}
		err = a.mutateCatalog(context.Background(), func() error { return cat.RemoveUser(context.Background(), u.ID) })
		if calls != 1 || (err != nil) != fail {
			t.Fatalf("runtime calls=%d failure=%v err=%v", calls, fail, err)
		}
		users, err := cat.ListUsers(context.Background(), s.ID)
		if err != nil {
			t.Fatal(err)
		}
		if (len(users) == 1) != fail {
			t.Fatal("catalog did not reflect runtime outcome")
		}
	}
}
