// support/support_test.go pins the V1
// support-bundle redaction contract.
package support

import (
	"strings"
	"testing"
)

func TestRedactorRedacts(t *testing.T) {
	r := NewRedactor(DefaultPatterns())
	in := "api_token=abcdef0123456789abcdef0123456789"
	out := r.Redact(in)
	if strings.Contains(out, "abcdef0123456789abcdef0123456789") {
		t.Fatalf("hex token not redacted: %q", out)
	}
	if !strings.Contains(out, RedactionTag) {
		t.Fatalf("redaction tag missing: %q", out)
	}
}

func TestRedactorRedactsBearer(t *testing.T) {
	r := NewRedactor(DefaultPatterns())
	in := "Authorization: Bearer abcdefghijklmnopqrstuvwxyz0123456789"
	out := r.Redact(in)
	if strings.Contains(out, "abcdefghijklmnopqrstuvwxyz0123456789") {
		t.Fatalf("bearer not redacted: %q", out)
	}
}

func TestRedactorRedactsVaultRef(t *testing.T) {
	r := NewRedactor(DefaultPatterns())
	in := "secret at vault://dns/cloudflare/api-token"
	out := r.Redact(in)
	if strings.Contains(out, "vault://") {
		t.Fatalf("vault ref not redacted: %q", out)
	}
}

func TestRedactorRedactsBasicAuth(t *testing.T) {
	r := NewRedactor(DefaultPatterns())
	in := "url: https://user:secret@example.com/api"
	out := r.Redact(in)
	if strings.Contains(out, "secret") {
		t.Fatalf("basic auth not redacted: %q", out)
	}
}

func TestRedactorAddsPattern(t *testing.T) {
	r := NewRedactor(nil)
	r.AddPattern(MustPattern("foo", `bar\d+`, "test"))
	if !strings.Contains(r.Redact("bar123"), RedactionTag) {
		t.Fatal("added pattern not applied")
	}
}

func TestBundleBuilderRedacts(t *testing.T) {
	r := NewRedactor(DefaultPatterns())
	bb := NewBundleBuilder("host-1", "v1.0.0", r, nil)
	bb.Add(File{Path: "/secret/token", Body: []byte("api_token=abcdef0123456789abcdef0123456789"), Redact: true})
	bb.Add(File{Path: "/plain/version", Body: []byte("duckos v1.0.0"), Redact: false})
	bundle := bb.Build()
	if len(bundle.Files) != 2 {
		t.Fatalf("files: %d", len(bundle.Files))
	}
	if strings.Contains(string(bundle.Files[0].Body), "abcdef") {
		t.Fatal("redacted file still contains secret")
	}
	if !strings.Contains(string(bundle.Files[1].Body), "duckos v1.0.0") {
		t.Fatal("plain file should not be redacted")
	}
	if len(bundle.Redaction) == 0 {
		t.Fatal("redaction pattern list empty")
	}
}

func TestBundleBuilderEmptyPath(t *testing.T) {
	r := NewRedactor(nil)
	bb := NewBundleBuilder("h", "v", r, nil)
	bb.Add(File{Path: "", Body: []byte("x")})
	if len(bb.Build().Files) != 0 {
		t.Fatal("empty path must be dropped")
	}
}

func TestBundleBuilderPathNormalize(t *testing.T) {
	r := NewRedactor(nil)
	bb := NewBundleBuilder("h", "v", r, nil)
	bb.Add(File{Path: "no-slash", Body: []byte("x")})
	if !strings.HasPrefix(bb.Build().Files[0].Path, "/") {
		t.Fatal("path not normalized")
	}
}
