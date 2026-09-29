package network

import (
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"syscall"
)

// ArtifactDirectory owns only explicitly rendered files in an existing root-
// controlled directory. It never adopts files merely because they share a
// prefix. This primitive is for the Agent's network adapter, not public API use.
type ArtifactDirectory struct {
	root     *os.Root
	validate func(map[string]string) error
}

func OpenArtifactDirectory(path string) (*ArtifactDirectory, error) {
	return openArtifacts(path, validateArtifacts)
}

// OpenDNSArtifacts is a fixed-role Agent store, not a general path-writing API.
// It permits exactly the dedicated DNS service configuration basename.
func OpenDNSArtifacts(path string) (*ArtifactDirectory, error) {
	return openArtifacts(path, func(files map[string]string) error {
		if len(files) > 1 {
			return errors.New("unexpected DNS artifact count")
		}
		for name, value := range files {
			if name != "dnsmasq.conf" || len(value) == 0 || len(value) > 256<<10 {
				return errors.New("invalid DNS artifact")
			}
		}
		return nil
	})
}

func openArtifacts(path string, validate func(map[string]string) error) (*ArtifactDirectory, error) {
	i, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !i.IsDir() || i.Mode().Perm()&0022 != 0 {
		return nil, errors.New("unsafe artifact directory")
	}
	if st, ok := i.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != os.Geteuid() {
		return nil, errors.New("artifact directory not owned by service")
	}
	r, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	return &ArtifactDirectory{root: r, validate: validate}, nil
}
func (d *ArtifactDirectory) Close() error { return d.root.Close() }

// Check verifies exact owned before-state and refuses collisions for newly
// introduced files. Unrelated files remain untouched and are not imported.
func (d *ArtifactDirectory) Check(before, next map[string]string) error {
	if err := d.validate(before); err != nil {
		return err
	}
	if err := d.validate(next); err != nil {
		return err
	}
	for _, files := range []map[string]string{before, next} {
		for name := range files {
			if _, err := d.root.Lstat(name + ".duckos-tmp"); !errors.Is(err, os.ErrNotExist) {
				return errors.New("staging residue requires transaction recovery")
			}
		}
	}
	for name, want := range before {
		got, err := d.read(name)
		if err != nil || got != want {
			return errors.New("managed artifact drift; explicit reconciliation required")
		}
	}
	for name := range next {
		if _, owned := before[name]; !owned {
			if _, err := d.root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
				return errors.New("candidate collides with an existing artifact")
			}
		}
	}
	return nil
}

// Replace checks every relevant path before writing. File replacements are
// individually durable; the surrounding transaction journal is responsible for
// recovery across interruption between files. It must be durable first.
func (d *ArtifactDirectory) Replace(before, next map[string]string) error {
	if err := d.Check(before, next); err != nil {
		return err
	}
	return d.replace(next, before)
}

// Restore admits only the exact before or candidate bytes (or an interrupted
// missing file) for each owned name. A third-party value is preserved and
// reported as a recovery failure, never silently overwritten.
func (d *ArtifactDirectory) Restore(before, candidate map[string]string) error {
	if err := d.validate(before); err != nil {
		return err
	}
	if err := d.validate(candidate); err != nil {
		return err
	}
	union := map[string]bool{}
	for n := range before {
		union[n] = true
	}
	for n := range candidate {
		union[n] = true
	}
	for name := range union {
		got, err := d.read(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		old, wasOld := before[name]
		next, wasNext := candidate[name]
		if (!wasOld || got != old) && (!wasNext || got != next) {
			return errors.New("artifact changed outside transaction; manual recovery required")
		}
	}
	// Check all staged files before touching any. Journaled names and prefix
	// checks admit interrupted writes, not arbitrary files in this directory.
	staged := []string{}
	for name := range union {
		temp := name + ".duckos-tmp"
		got, err := d.read(temp)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		old, wasOld := before[name]
		next, wasNext := candidate[name]
		if (!wasOld || !strings.HasPrefix(old, got)) && (!wasNext || !strings.HasPrefix(next, got)) {
			return errors.New("unrecognized staging file; manual recovery required")
		}
		staged = append(staged, temp)
	}
	for _, name := range staged {
		if err := d.root.Remove(name); err != nil {
			return err
		}
	}
	return d.replace(before, candidate)
}
func (d *ArtifactDirectory) replace(next, old map[string]string) error {
	names := make([]string, 0, len(next))
	for name := range next {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := next[name]
		if err := d.write(name, value); err != nil {
			return err
		}
	}
	for name := range old {
		if _, keep := next[name]; !keep {
			if err := d.root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return d.sync()
}
func (d *ArtifactDirectory) read(name string) (string, error) {
	i, err := d.root.Lstat(name)
	if err != nil {
		return "", err
	}
	if !i.Mode().IsRegular() || i.Mode().Perm()&0022 != 0 || i.Size() > 256<<10 {
		return "", errors.New("unsafe managed artifact")
	}
	if st, ok := i.Sys().(*syscall.Stat_t); !ok || st.Nlink != 1 || int(st.Uid) != os.Geteuid() {
		return "", errors.New("unsafe artifact ownership or hardlink")
	}
	f, err := d.root.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	j, err := f.Stat()
	if err != nil || !os.SameFile(i, j) {
		return "", errors.New("artifact replaced during read")
	}
	b, err := io.ReadAll(io.LimitReader(f, (256<<10)+1))
	if err != nil {
		return "", err
	}
	if len(b) > 256<<10 {
		return "", errors.New("artifact exceeds limit")
	}
	return string(b), nil
}
func (d *ArtifactDirectory) write(name, value string) error {
	// Exclusive creation avoids following a planted or crashed temporary path.
	// Fixed suffix bounds staging growth to one file per owned artifact.
	temp := name + ".duckos-tmp"
	if _, err := d.root.Lstat(temp); !errors.Is(err, os.ErrNotExist) {
		return errors.New("artifact staging residue requires recovery")
	}
	f, err := d.root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(value)
	if err == nil {
		err = f.Chmod(0644)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = d.root.Rename(temp, name); err != nil {
		return err
	}
	return d.sync()
}
func (d *ArtifactDirectory) sync() error {
	f, err := d.root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func validateArtifacts(files map[string]string) error {
	if len(files) > 256 {
		return errors.New("too many artifacts")
	}
	bytes := 0
	for name, contents := range files {
		if len(name) > 100 || !strings.HasPrefix(name, "10-duckos-") || (!strings.HasSuffix(name, ".network") && !strings.HasSuffix(name, ".netdev")) || strings.ContainsAny(name, "/\\\n\r\x00") {
			return errors.New("invalid managed artifact name")
		}
		stem := strings.TrimPrefix(name, "10-duckos-")
		if strings.HasSuffix(stem, ".network") {
			stem = strings.TrimSuffix(stem, ".network")
		} else {
			stem = strings.TrimSuffix(stem, ".netdev")
		}
		if !identifier.MatchString(stem) || len(contents) > 256<<10 {
			return errors.New("invalid artifact identity or size")
		}
		bytes += len(contents)
	}
	if bytes > 2<<20 {
		return errors.New("artifact set exceeds size bound")
	}
	return nil
}
