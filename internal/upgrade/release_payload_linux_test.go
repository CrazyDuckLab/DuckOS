//go:build linux

package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

// Full releases contain executable launchers in addition to API and Agent.
// Exercise the production Stage path, including signature and native probes.
func TestFullReleasePayloadExecutableModesAndCompression(t *testing.T) {
	priv, pub := mustKey(t)
	base := nativeCandidate(t, priv, "vfull", false)
	pkg, err := Decode(base.Payload, multiKeyVerifier([]ed25519.PublicKey{pub}))
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	tr := tar.NewReader(bytes.NewReader(pkg.Payload))
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		if e = tw.WriteHeader(h); e != nil {
			t.Fatal(e)
		}
		if _, e = io.Copy(tw, tr); e != nil {
			t.Fatal(e)
		}
	}
	helper := []byte("#!/bin/sh\nexit 0\n")
	if err = tw.WriteHeader(&tar.Header{Name: "bin/duck-xray-launcher", Mode: 04777, Size: int64(len(helper))}); err != nil {
		t.Fatal(err)
	}
	if _, err = tw.Write(helper); err != nil {
		t.Fatal(err)
	}
	if err = tw.Close(); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name       string
		compressed bool
		mask       int
	}{{"tar", false, 0022}, {"tar-gzip", true, 0022}, {"private-umask-tar", false, 0077}, {"private-umask-gzip", true, 0077}} {
		compressed := fixture.compressed
		name := fixture.name
		payload := raw.Bytes()
		if compressed {
			var z bytes.Buffer
			w := gzip.NewWriter(&z)
			if _, err = w.Write(payload); err != nil {
				t.Fatal(err)
			}
			if err = w.Close(); err != nil {
				t.Fatal(err)
			}
			payload = z.Bytes()
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			oldMask := syscall.Umask(fixture.mask)
			defer syscall.Umask(oldMask)
			store, err := NewReleaseStore(filepath.Join(dir, "store"), 64<<20)
			if err != nil {
				t.Fatal(err)
			}
			driver := &processRuntime{root: filepath.Join(dir, "installed"), socket: filepath.Join(dir, "live.sock")}
			s, err := OpenSupervisor(store, []ed25519.PublicKey{pub}, runtime.GOARCH, driver.root, driver)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if err = s.Stage(signedArchive(t, priv, "vfull", payload)); err != nil {
				t.Fatal(err)
			}
			binDir := filepath.Join(driver.root, "releases", "vfull", "bin")
			di, err := os.Stat(binDir)
			if err != nil || di.Mode().Perm() != 0755 {
				t.Fatalf("release directory inaccessible under private umask: %v %v", di, err)
			}
			file := filepath.Join(driver.root, "releases", "vfull", "bin", "duck-xray-launcher")
			st, err := os.Stat(file)
			if err != nil {
				t.Fatal(err)
			}
			if st.Mode().Perm() != 0755 || st.Mode()&os.ModeSetuid != 0 {
				t.Fatalf("unsafe or non-executable helper mode: %v", st.Mode())
			}
			if err = os.Chmod(file, 0644); err != nil {
				t.Fatal(err)
			}
			if _, err = s.verifiedBuild("vfull"); err == nil {
				t.Fatal("helper executable-bit tampering accepted")
			}
		})
	}
}
