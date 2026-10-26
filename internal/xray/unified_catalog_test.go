package xray

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUnifiedServerDeletionPreservesUnrelatedCatalog(t *testing.T) {
	for _, failCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "committed", true: "disk-failure"}[failCommit], func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			cat, err := OpenFileCatalog(dir)
			if err != nil {
				t.Fatal(err)
			}
			initial := fileState{
				Servers:       []Server{{ID: "server-a"}, {ID: "server-b"}},
				Users:         []User{{ID: "user-a", ServerID: "server-a"}, {ID: "user-b", ServerID: "server-b"}},
				Profiles:      []Profile{{ID: "profile-a", UserID: "user-a"}, {ID: "profile-b", UserID: "user-b"}},
				Subscriptions: []Subscription{{ID: "shared", ProfileIDs: []string{"profile-a", "profile-b"}}, {ID: "other", ProfileIDs: []string{"profile-b"}}},
			}
			if err := cat.Restore(ctx, initial); err != nil {
				t.Fatal(err)
			}
			if failCommit {
				if err := os.Mkdir(filepath.Join(dir, "catalog.json.tmp"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			err = cat.RemoveServer(ctx, "server-a")
			if failCommit && err == nil {
				t.Fatal("disk failure reported success")
			}
			if !failCommit && err != nil {
				t.Fatal(err)
			}
			memory, err := cat.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenFileCatalog(dir)
			if err != nil {
				t.Fatal(err)
			}
			disk, err := reopened.Snapshot(ctx)
			if err != nil || !reflect.DeepEqual(memory, disk) {
				t.Fatal("memory and disk disagree after deletion", err)
			}
			if failCommit {
				if !reflect.DeepEqual(memory, initial) {
					t.Fatal("failed deletion mutated authoritative catalog")
				}
				return
			}
			if len(memory.Servers) != 1 || memory.Servers[0].ID != "server-b" || len(memory.Users) != 1 || memory.Users[0].ID != "user-b" || len(memory.Profiles) != 1 || memory.Profiles[0].ID != "profile-b" {
				t.Fatalf("unrelated catalog deleted or orphan retained: %+v", memory)
			}
			if len(memory.Subscriptions) != 2 {
				t.Fatal("unrelated subscriptions were deleted")
			}
			for _, sub := range memory.Subscriptions {
				if !reflect.DeepEqual(sub.ProfileIDs, []string{"profile-b"}) {
					t.Fatalf("wrong surviving profile set: %+v", sub)
				}
			}
		})
	}
}
