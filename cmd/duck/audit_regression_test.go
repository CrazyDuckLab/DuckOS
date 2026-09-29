package main

import (
	"net/http"
	"testing"
)

func TestAuditHTTPRequiresLiteralLoopback(t *testing.T) {
	for _, endpoint := range []string{"http://192.0.2.1:8080", "http://example.invalid", "http://user:pass@127.0.0.1:8080", "http://127.0.0.1:8080?token=x"} {
		_, err := newState(&commonFlags{endpoint: endpoint, insecure: true, tokenFile: "/definitely-not-a-token"})
		if err == nil {
			t.Fatal("unsafe endpoint accepted")
		}
		// Endpoint rejection must precede reading any token.
		if err != nil && len(err.Error()) >= 10 && err.Error()[:10] == "token file" {
			t.Errorf("endpoint accepted until token read: %s", endpoint)
		}
	}
}
func TestAuditClientRejectsRedirects(t *testing.T) {
	c := newClient("http://127.0.0.1:8080", "", false)
	if c.CheckRedirect == nil {
		t.Fatal("redirect following enabled")
	}
	if err := c.CheckRedirect(&http.Request{}, nil); err != http.ErrUseLastResponse {
		t.Fatal("redirect not refused")
	}
}
