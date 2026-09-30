package upgrade

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

func ownedDirectory(dir string, mode os.FileMode) error {
	if err := os.MkdirAll(dir, mode); err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || resolved != dir {
		return errors.New("upgrade: symlink in trusted root")
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || int(owner.Uid) != os.Geteuid() || info.Mode().Perm()&0022 != 0 {
		return errors.New("upgrade: unsafe trusted directory")
	}
	return nil
}

func walkPayload(pkg *Package, visit func(string, []byte) error) error {
	r := tar.NewReader(bytes.NewReader(pkg.Payload))
	seen := map[string]bool{}
	var size int64
	for count := 0; ; count++ {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(h.Name, "/")
		if count >= 10000 || name == "" || path.Clean(name) != name || strings.HasPrefix(name, "/") || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "\\") || seen[name] {
			return errors.New("upgrade: invalid archive path")
		}
		seen[name] = true
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if h.Typeflag != tar.TypeReg || h.Size < 0 || h.Size > MaxPayloadBytes {
			return errors.New("upgrade: unsupported archive entry")
		}
		size += h.Size
		if size > MaxPayloadBytes {
			return errors.New("upgrade: expanded payload too large")
		}
		b, err := io.ReadAll(io.LimitReader(r, h.Size+1))
		if err != nil || int64(len(b)) != h.Size {
			return errors.New("upgrade: incomplete archive file")
		}
		if err := visit(name, b); err != nil {
			return err
		}
	}
	for _, name := range []string{"bin/duck-agent", "bin/duck-api", "BUILD-INFO.json"} {
		if !seen[name] {
			return errors.New("upgrade: required release artifact missing")
		}
	}
	return nil
}

func (s *Supervisor) prepare(id string, pkg *Package) error {
	if !validStoredReleaseID(id) {
		return errors.New("upgrade: invalid release identity")
	}
	dest := filepath.Join(s.root, "releases", id)
	if _, err := os.Lstat(dest); !errors.Is(err, os.ErrNotExist) {
		return errors.New("upgrade: prepared release already exists")
	}
	tmp, err := os.MkdirTemp(filepath.Join(s.root, "releases"), ".candidate-")
	if err != nil {
		return err
	}
	// Only this newly-created staging tree may be discarded on failure.
	defer os.RemoveAll(tmp)
	err = walkPayload(pkg, func(name string, b []byte) error {
		file := filepath.Join(tmp, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if name == "bin/duck-agent" || name == "bin/duck-api" {
			if len(b) < 4 || !bytes.Equal(b[:4], []byte{0x7f, 'E', 'L', 'F'}) {
				return errors.New("upgrade: native ELF executable required")
			}
			mode = 0755
		}
		f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err = f.Write(b); err != nil {
			return err
		}
		return f.Sync()
	})
	if err != nil {
		return err
	}
	build, err := readBuild(tmp, pkg)
	if err != nil {
		return err
	}
	if err := nativeBuildProbe(tmp, build); err != nil {
		return err
	}
	if err := filepath.WalkDir(tmp, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return syncReleaseDir(p)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0755); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	return syncReleaseDir(filepath.Dir(dest))
}

func readBuild(dir string, pkg *Package) (BuildIdentity, error) {
	var b BuildIdentity
	raw, err := os.ReadFile(filepath.Join(dir, "BUILD-INFO.json"))
	if err != nil || len(raw) > 65536 {
		return b, errors.New("upgrade: invalid build identity")
	}
	if json.Unmarshal(raw, &b) != nil || b.Version != pkg.Manifest.TargetVersion || b.Target != "linux/"+archForTarget(pkg.Manifest.Architecture) || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(b.Commit) {
		return b, errors.New("upgrade: signed build identity mismatch")
	}
	return b, nil
}

// archForTarget maps a runtime.GOARCH value to the canonical DuckOS
// BUILD-INFO "target" arch suffix. Only GOARCH values that disagree
// with the public target naming need an entry. Add new mappings here
// when supporting a new CPU architecture with a divergent GOARCH.
func archForTarget(goarch string) string {
	switch goarch {
	case "arm":
		return "armv7"
	default:
		return goarch
	}
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 4096 {
		return 0, errors.New("upgrade: native output exceeds limit")
	}
	return b.Buffer.Write(p)
}

func nativeBuildProbe(dir string, build BuildIdentity) error {
	for _, name := range []string{"duck-agent", "duck-api"} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, filepath.Join(dir, "bin", name), "--version")
		cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C"}
		cmd.WaitDelay = time.Second
		var output boundedOutput
		cmd.Stdout = &output
		cmd.Stderr = io.Discard
		err := cmd.Run()
		cancel()
		if err != nil || strings.TrimSpace(output.String()) != fmt.Sprintf("DuckOS %s (%s)", build.Version, build.Commit) {
			return errors.New("upgrade: native executable/build verification failed")
		}
	}
	return nil
}

func (s *Supervisor) verifiedBuild(id string) (BuildIdentity, error) {
	if err := s.store.VerifyFingerprint(id); err != nil {
		return BuildIdentity{}, err
	}
	raw, err := s.store.ReadPayload(id)
	if err != nil {
		return BuildIdentity{}, err
	}
	pkg, err := Decode(raw, multiKeyVerifier(s.o.keys))
	if err != nil {
		return BuildIdentity{}, err
	}
	dir := filepath.Join(s.root, "releases", id)
	if err := ownedDirectory(dir, 0755); err != nil {
		return BuildIdentity{}, err
	}
	err = walkPayload(pkg, func(name string, want []byte) error {
		full := filepath.Join(dir, filepath.FromSlash(name))
		resolved, err := filepath.EvalSymlinks(full)
		if err != nil || resolved != full {
			return errors.New("upgrade: release path changed")
		}
		f, err := os.Open(full)
		if err != nil {
			return err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() != int64(len(want)) {
			return errors.New("upgrade: release file changed")
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		expected := sha256.Sum256(want)
		if !bytes.Equal(h.Sum(nil), expected[:]) {
			return errors.New("upgrade: release digest changed")
		}
		return nil
	})
	if err != nil {
		return BuildIdentity{}, err
	}
	return readBuild(dir, pkg)
}
