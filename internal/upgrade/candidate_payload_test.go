package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"testing"
)

func TestCompressedReleasePayloadBoundsAndTrailer(t *testing.T) {
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for _, name := range []string{"bin/duck-agent", "bin/duck-api", "BUILD-INFO.json"} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: 1}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	compress := func(b []byte) []byte {
		var out bytes.Buffer
		w := gzip.NewWriter(&out)
		if _, err := w.Write(b); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	valid := compress(raw.Bytes())
	badCRC := bytes.Clone(valid)
	badCRC[len(badCRC)-8] ^= 1
	cases := []struct {
		name        string
		payload     []byte
		total, file int64
		ok          bool
	}{
		{"legacy-tar", raw.Bytes(), 8192, 64, true},
		{"gzip", valid, 8192, 64, true},
		{"exact-expanded-bound", valid, int64(raw.Len()), 64, true},
		{"expansion-limit", valid, int64(raw.Len() - 1), 64, false},
		{"bad-crc", badCRC, 8192, 64, false},
		{"truncated-trailer", valid[:len(valid)-1], 8192, 64, false},
		{"truncated-header", []byte{0x1f, 0x8b}, 8192, 64, false},
		{"trailing-stream", append(bytes.Clone(valid), valid...), 16384, 64, false},
		{"trailing-junk", append(bytes.Clone(valid), byte(1)), 8192, 64, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := walkPayloadBounded(&Package{Payload: c.payload}, c.total, c.file, func(string, []byte, os.FileMode) error { return nil })
			if (err == nil) != c.ok {
				t.Fatalf("accepted=%v want=%v: %v", err == nil, c.ok, err)
			}
		})
	}
	var oversized bytes.Buffer
	w := tar.NewWriter(&oversized)
	if err := w.WriteHeader(&tar.Header{Name: "bin/duck-agent", Size: 65, Mode: 0755}); err != nil {
		t.Fatal(err)
	}
	// A declared large file must be rejected before reading/allocating its body.
	if err := walkPayloadBounded(&Package{Payload: compress(oversized.Bytes())}, 8192, 64, func(string, []byte, os.FileMode) error { t.Fatal("oversized file visited"); return nil }); err == nil {
		t.Fatal("oversized file accepted")
	}
}

func TestCompressedPayloadSignatureStillBindsEveryByte(t *testing.T) {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := io.WriteString(w, "not a tar"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	_, raw, _, verifier := buildPackage(t, b.Bytes())
	if _, err := Decode(raw, verifier); err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 1
	if _, err := Decode(raw, verifier); err == nil {
		t.Fatal("compressed payload tampering accepted")
	}
}
