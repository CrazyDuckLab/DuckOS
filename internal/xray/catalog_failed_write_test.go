package xray

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func reacceptState() fileState {
	return fileState{
		Servers:       []Server{{ID: "server-a", Name: "a", Enabled: true}, {ID: "server-b", Name: "b", Enabled: true}},
		Users:         []User{{ID: "user-a", ServerID: "server-a", Enabled: true}, {ID: "user-b", ServerID: "server-b", Enabled: true}},
		Profiles:      []Profile{{ID: "profile-a", UserID: "user-a"}, {ID: "profile-b", UserID: "user-b"}},
		Subscriptions: []Subscription{{ID: "shared", ProfileIDs: []string{"profile-a", "profile-b"}}},
	}
}
func reacceptMutators() map[string]func(*FileCatalog) error {
	ctx := context.Background()
	return map[string]func(*FileCatalog) error{
		"upsert-server": func(c *FileCatalog) error { return c.UpsertServer(ctx, Server{ID: "server-a", Name: "changed"}) },
		"remove-server": func(c *FileCatalog) error { return c.RemoveServer(ctx, "server-a") },
		"upsert-user": func(c *FileCatalog) error {
			return c.UpsertUser(ctx, User{ID: "user-a", ServerID: "server-a", Name: "changed"})
		},
		"remove-user": func(c *FileCatalog) error { return c.RemoveUser(ctx, "user-a") },
		"upsert-profile": func(c *FileCatalog) error {
			return c.UpsertProfile(ctx, Profile{ID: "profile-a", UserID: "user-a", Name: "changed"})
		},
		"remove-profile": func(c *FileCatalog) error { return c.RemoveProfile(ctx, "profile-a") },
		"upsert-subscription": func(c *FileCatalog) error {
			return c.UpsertSubscription(ctx, Subscription{ID: "shared", Name: "changed"})
		},
		"remove-subscription": func(c *FileCatalog) error { return c.RemoveSubscription(ctx, "shared") },
	}
}

// Filesystem oracle, independent of the new writer's fault-injection seam.
// This test also runs against the exact reviewed source before remediation.
func TestReacceptCatalogFailedWritePreservesEveryMutation(t *testing.T) {
	for name, mutate := range reacceptMutators() {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			c, err := OpenFileCatalog(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := c.Restore(ctx, reacceptState()); err != nil {
				t.Fatal(err)
			}
			before, err := c.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(dir, "catalog.json.tmp"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := mutate(c); err == nil {
				t.Fatal("failed write returned success")
			}
			memory, err := c.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenFileCatalog(dir)
			if err != nil {
				t.Fatal(err)
			}
			disk, err := reopened.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, memory) || !reflect.DeepEqual(memory, disk) {
				t.Fatal("failed write changed authoritative memory or disk")
			}
		})
	}
}
