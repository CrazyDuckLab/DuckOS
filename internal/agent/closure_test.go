// Package agent / closure_test.go: F-02 trusted-peer scope regression
// for the final security closure (2026-10-24 round).
package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

// F-02: trusted peer metadata may be applied ONLY on the canonical
// authenticated node-auth route. Non-node-auth IPC requests keep the
// original Unix IPC RemoteAddr and never see the metadata header;
// spoofable client headers are ignored. Node-auth keeps the
// propagation (with IPC authentication mandatory).
func TestClosurePeerMetadataIsScopedToNodeAuth(t *testing.T) {
	const key = "closure-peer-boundary-key-01234567890123456789"
	for _, path := range []string{"/xray/v1/servers", "/xray/v1/center/nodes", "/xray/v1/center/node-authx/node/pull"} {
		t.Run(path, func(t *testing.T) {
			reached := false
			peer := ""
			header := ""
			source := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				peer = r.RemoteAddr
				header = r.Header.Get(NodeAuthPeerHeader)
				w.WriteHeader(http.StatusNoContent)
			})
			host := NewXrayHost(source, identity.ForwardedAccess{Secret: key}, nil)
			r := httptest.NewRequest("GET", XrayPath+path, nil)
			r.RemoteAddr = "@"
			r.Header.Set(NodeAuthPeerHeader, "192.0.2.99")
			r.Header.Set("Authorization", "Bearer "+identity.ForwardedToken(key, identity.User{Username: "duck-api-node-auth", Role: identity.ReadOnly}))
			w := httptest.NewRecorder()
			host.ServeHTTP(w, r)
			if !reached {
				t.Fatal("unexpected rejection before metadata inspection")
			}
			if header != "" {
				t.Fatal("trusted metadata header was not removed")
			}
			if peer != "@" {
				t.Fatalf("non-node-auth IPC request applied trusted peer metadata: %s", peer)
			}
		})
	}
	// The canonical node-auth route still propagates the trusted peer
	// (after mandatory IPC authorization) and strips the header.
	reached := false
	peer := ""
	header := ""
	source := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		peer = r.RemoteAddr
		header = r.Header.Get(NodeAuthPeerHeader)
	})
	host := NewXrayHost(source, identity.ForwardedAccess{Secret: key}, nil)
	r := httptest.NewRequest("POST", XrayPath+"/xray/v1/center/node-auth/node/pull", nil)
	r.RemoteAddr = "@"
	r.Header.Set(NodeAuthPeerHeader, "192.0.2.99")
	r.Header.Set("Authorization", "Bearer "+identity.ForwardedToken(key, identity.User{Username: "duck-api-node-auth", Role: identity.ReadOnly}))
	w := httptest.NewRecorder()
	host.ServeHTTP(w, r)
	if !reached {
		t.Fatal("node-auth route unexpectedly rejected")
	}
	if header != "" {
		t.Fatal("node-auth request forwarded the raw metadata header")
	}
	if peer != "192.0.2.99" {
		t.Fatalf("node-auth route lost the trusted peer propagation: %s", peer)
	}
}

// The node-auth route never bypasses the API→Agent IPC
// authentication, with or without peer metadata.
func TestClosureNodeAuthPeerDoesNotBypassIPCAuthentication(t *testing.T) {
	called := false
	host := NewXrayHost(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }),
		identity.ForwardedAccess{Secret: "closure-boundary-key"}, nil)
	for _, token := range []string{"", "fake-forwarded-token", "closure-node-credential"} {
		r := httptest.NewRequest("POST", XrayPath+"/xray/v1/center/node-auth/node/pull", nil)
		r.Header.Set(NodeAuthPeerHeader, "192.0.2.99")
		r.Header.Set("X-Node-Credential", "closure-node-credential")
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		host.ServeHTTP(w, r)
		if w.Code != 403 || called {
			t.Fatal("peer metadata bypassed IPC auth")
		}
	}
}
