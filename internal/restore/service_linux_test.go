//go:build linux

package restore

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func fixture(t *testing.T) (*Service, string, Request) {
	t.Helper()
	dir := t.TempDir()
	for _, p := range []string{"source/b1", "live", "state"} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range []string{"a", "b", "untouched"} {
		if err := os.WriteFile(filepath.Join(dir, "source/b1", k), []byte("new-"+k), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "live", k), []byte("old-"+k), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Open(filepath.Join(dir, "source"), filepath.Join(dir, "live"), filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir, Request{ID: strings.Repeat("1", 32), Expected: strings.Repeat("2", 64), Backup: "b1", Keys: []string{"a", "b"}}
}
func checkFile(t *testing.T, name, want string) {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil || string(b) != want {
		t.Fatalf("oracle %s: %q %v", name, b, err)
	}
}

func TestH09SelectiveRestoreAndReplay(t *testing.T) {
	s, dir, r := fixture(t)
	r.Keys = []string{"a"}
	result, err := s.Execute(context.Background(), r)
	if err != nil || result.Outcome != "applied" {
		t.Fatalf("%+v %v", result, err)
	}
	checkFile(t, filepath.Join(dir, "live/a"), "new-a")
	checkFile(t, filepath.Join(dir, "live/b"), "old-b")
	if err := os.WriteFile(filepath.Join(dir, "source/b1/a"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Execute(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	checkFile(t, filepath.Join(dir, "live/a"), "new-a")
	r.Keys = []string{"b"}
	if _, err := s.Execute(context.Background(), r); err == nil {
		t.Fatal("conflicting ID accepted")
	}
}

func TestH09MissingParentRefusedBeforeAnyWrite(t *testing.T) {
	s, dir, r := fixture(t)
	if err := os.Mkdir(filepath.Join(dir, "source/b1/missing"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "source/b1/missing/b"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	r.Keys = []string{"a", "missing/b"}
	if _, err := s.Execute(context.Background(), r); err == nil {
		t.Fatal("missing managed parent admitted")
	}
	checkFile(t, filepath.Join(dir, "live/a"), "old-a")
	if s.journal.Current != nil {
		t.Fatal("unapplicable selection left pending restore")
	}
}

func TestH09Confinement(t *testing.T) {
	for _, kind := range []string{"child", "source", "dotdot", "absolute", "foreign-root"} {
		t.Run(kind, func(t *testing.T) {
			s, dir, r := fixture(t)
			outside := filepath.Join(dir, "outside")
			if err := os.WriteFile(outside, []byte("untouched"), 0600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "child":
				os.Remove(filepath.Join(dir, "live/a"))
				if err := os.Symlink(outside, filepath.Join(dir, "live/a")); err != nil {
					t.Fatal(err)
				}
			case "source":
				os.Remove(filepath.Join(dir, "source/b1/a"))
				if err := os.Symlink(outside, filepath.Join(dir, "source/b1/a")); err != nil {
					t.Fatal(err)
				}
			case "dotdot":
				r.Keys = []string{"../outside"}
			case "absolute":
				r.Keys = []string{outside}
			case "foreign-root":
				r.Backup = "../outside"
			}
			if _, err := s.Execute(context.Background(), r); err == nil {
				t.Fatal("unsafe target accepted")
			}
			checkFile(t, outside, "untouched")
		})
	}
}

func TestH09DescriptorFallbackConfinement(t *testing.T) {
	s, dir, _ := fixture(t)
	outside := filepath.Join(dir, "outside")
	if err := os.WriteFile(outside, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "live/link")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"link", "../outside", outside} {
		fd, err := walkNoFollow(int(s.live.f.Fd()), name, unix.O_RDONLY, true)
		if err == nil {
			unix.Close(fd)
			t.Fatalf("fallback accepted %q", name)
		}
	}
	fd, err := walkNoFollow(int(s.live.f.Fd()), "a", unix.O_RDONLY, true)
	if err != nil {
		t.Fatal(err)
	}
	unix.Close(fd)
	checkFile(t, outside, "untouched")
}

func TestH09PartialFailureAndCanceledApplyUndo(t *testing.T) {
	for _, failure := range []string{"io", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			s, dir, r := fixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.afterWrite = func(i int) error {
				if i == 0 {
					if failure == "cancel" {
						cancel()
						return nil
					}
					return errors.New("injected partial failure")
				}
				return nil
			}
			result, err := s.Execute(ctx, r)
			if err != nil || result.Outcome != "rolled-back" {
				t.Fatalf("%+v %v", result, err)
			}
			checkFile(t, filepath.Join(dir, "live/a"), "old-a")
			checkFile(t, filepath.Join(dir, "live/b"), "old-b")
			s.Close()
			reopened, err := Open(filepath.Join(dir, "source"), filepath.Join(dir, "live"), filepath.Join(dir, "state"))
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			result, err = reopened.Execute(context.Background(), r)
			if err != nil || result.Outcome != "rolled-back" {
				t.Fatalf("terminal outcome lost: %+v %v", result, err)
			}
		})
	}
}

func TestH09SymlinkSwapBeforeSecondWrite(t *testing.T) {
	s, dir, r := fixture(t)
	outside := filepath.Join(dir, "outside")
	os.WriteFile(outside, []byte("untouched"), 0600)
	s.afterWrite = func(i int) error {
		if i == 0 {
			if err := os.Remove(filepath.Join(dir, "live/b")); err != nil {
				return err
			}
			return os.Symlink(outside, filepath.Join(dir, "live/b"))
		}
		return nil
	}
	result, err := s.Execute(context.Background(), r)
	if err == nil || result.Outcome != "recovery-required" {
		t.Fatalf("swap must fail closed, including unsafe undo: %+v %v", result, err)
	}
	checkFile(t, outside, "untouched")
	checkFile(t, filepath.Join(dir, "live/a"), "old-a")
}

func TestH09CrashHelper(t *testing.T) {
	dir := os.Getenv("DUCKOS_RESTORE_CRASH")
	if dir == "" {
		t.Skip("subprocess helper")
	}
	s, err := Open(filepath.Join(dir, "source"), filepath.Join(dir, "live"), filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	s.afterWrite = func(i int) error {
		if i == 0 {
			if err := os.WriteFile(filepath.Join(dir, "written"), []byte("ready"), 0600); err != nil {
				return err
			}
			select {}
		}
		return nil
	}
	_, err = s.Execute(context.Background(), Request{ID: strings.Repeat("1", 32), Expected: strings.Repeat("2", 64), Backup: "b1", Keys: []string{"a", "b"}})
	t.Fatal("helper returned", err)
}

func TestH09KilledWriterAutomaticallyUndone(t *testing.T) {
	s, dir, r := fixture(t)
	s.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestH09CrashHelper$", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "DUCKOS_RESTORE_CRASH="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "written")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("live write boundary not reached")
		}
		time.Sleep(10 * time.Millisecond)
	}
	checkFile(t, filepath.Join(dir, "live/a"), "new-a")
	checkFile(t, filepath.Join(dir, "live/b"), "old-b")
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	recovered, err := Open(filepath.Join(dir, "source"), filepath.Join(dir, "live"), filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	checkFile(t, filepath.Join(dir, "live/a"), "old-a")
	checkFile(t, filepath.Join(dir, "live/b"), "old-b")
	result, err := recovered.Execute(context.Background(), r)
	if err != nil || result.Outcome != "rolled-back" || recovered.journal.Current != nil {
		t.Fatalf("ambiguous recovery outcome: %+v %v", result, err)
	}
}

func TestH09ReconcileUnknownPreventsLateExecution(t *testing.T) {
	s, dir, r := fixture(t)
	result, err := s.Reconcile(r)
	if err != nil || result.Outcome != "rolled-back" {
		t.Fatal(result, err)
	}
	result, err = s.Execute(context.Background(), r)
	if err != nil || result.Outcome != "rolled-back" {
		t.Fatal(result, err)
	}
	checkFile(t, filepath.Join(dir, "live/a"), "old-a")
	s.Close()
	if err := os.WriteFile(filepath.Join(dir, "state/journal.json"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if service, err := Open(filepath.Join(dir, "source"), filepath.Join(dir, "live"), filepath.Join(dir, "state")); err == nil {
		service.Close()
		t.Fatal("corrupt journal accepted")
	}
	checkFile(t, filepath.Join(dir, "live/a"), "old-a")
}

func TestH09MountEscape(t *testing.T) {
	if os.Getenv("DUCKOS_RESTORE_MOUNT_NS") == "" {
		if os.Geteuid() != 0 {
			t.Skip("root required for private mount namespace")
		}
		command := exec.Command("unshare", "--mount", "--propagation", "private", os.Args[0], "-test.run=^TestH09MountEscape$", "-test.timeout=20s")
		command.Env = append(os.Environ(), "DUCKOS_RESTORE_MOUNT_NS=1")
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("private mount fixture: %v %s", err, out)
		}
		return
	}
	s, dir, r := fixture(t)
	outside := filepath.Join(dir, "outside")
	target := filepath.Join(dir, "live/mounted")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(outside, "a"), []byte("untouched"), 0600)
	if err := syscall.Mount(outside, target, "", syscall.MS_BIND, ""); err != nil {
		t.Fatal(err)
	}
	defer syscall.Unmount(target, 0)
	r.Keys = []string{"mounted/a"}
	if fd, err := walkNoFollow(int(s.live.f.Fd()), "mounted/a", unix.O_RDONLY, true); err == nil {
		unix.Close(fd)
		t.Fatal("descriptor fallback crossed bind mount")
	}
	if _, err := s.Execute(context.Background(), r); err == nil {
		t.Fatal("mount escape accepted")
	}
	checkFile(t, filepath.Join(outside, "a"), "untouched")
}

func TestH09JournalFailureNeverClaimsCommit(t *testing.T) {
	for _, failureAt := range []int{1, 2} {
		t.Run(map[int]string{1: "before-live-write", 2: "terminal-commit"}[failureAt], func(t *testing.T) {
			s, dir, r := fixture(t)
			writes := 0
			s.beforePersist = func() error {
				writes++
				if writes == failureAt {
					return errors.New("injected journal failure")
				}
				return nil
			}
			result, err := s.Execute(context.Background(), r)
			if err == nil || result.Outcome != "recovery-required" {
				t.Fatal("uncertain journal reported terminal success", result, err)
			}
			if failureAt == 1 {
				checkFile(t, filepath.Join(dir, "live/a"), "old-a")
			}
			s.Close()
			reopened, err := Open(filepath.Join(dir, "source"), filepath.Join(dir, "live"), filepath.Join(dir, "state"))
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			checkFile(t, filepath.Join(dir, "live/a"), "old-a")
			checkFile(t, filepath.Join(dir, "live/b"), "old-b")
			result, err = reopened.Reconcile(r)
			if err != nil || result.Outcome != "rolled-back" {
				t.Fatal("failed commit did not recover deterministically", result, err)
			}
		})
	}
}
