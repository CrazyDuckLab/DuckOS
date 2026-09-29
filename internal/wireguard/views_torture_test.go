// Package wireguard torture: §G12 view redaction regression.
package wireguard

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

func makeTestServer() Server {
	now := time.Now().UTC()
	return Server{
		ID:            "11111111111111111111111111111111",
		Name:          "primary",
		Interface:     "wg0",
		ListenPort:    51820,
		Addresses:     []string{"10.0.0.1/24"},
		DNS:           []string{"1.1.1.1"},
		MTU:           1420,
		PrivateKeyRef: KeyRef{VaultID: "vault-1", Revision: "rev-1", KeyID: "key-1"},
		PublicKey:     "pub-key-1",
		Peers: []Peer{{
			ID:         "22222222222222222222222222222222",
			ServerID:   "11111111111111111111111111111111",
			Name:       "alice",
			PublicKey:  "pub-peer-alpha",
			AllowedIPs: []string{"10.0.0.2/32"},
			Intent:     IntentFullTunnel,
			Enabled:    true,
			CreatedAt:  now, UpdatedAt: now,
		}},
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestTorture_ViewServerRedactsPrivateKey(t *testing.T) {
	s := makeTestServer()
	v := ViewServer(s, object.PermissionSet{Observe: true, Configure: true})
	if !v.HasPrivateKey {
		t.Errorf("HasPrivateKey must be true")
	}
	if v.PublicKeyDigest == s.PublicKey {
		t.Errorf("public key leaked into view")
	}
	js, _ := json.Marshal(v)
	for _, leak := range []string{"vault-1", "key-1", s.PrivateKeyRef.Revision, s.PublicKey} {
		if strings.Contains(string(js), leak) {
			t.Errorf("server view leaks %q", leak)
		}
	}
}

func TestTorture_ViewPeerRedacts(t *testing.T) {
	s := makeTestServer()
	peer := s.Peers[0]
	peer.PresharedKeyRef = &KeyRef{VaultID: "vault-1", Revision: "rev-1", KeyID: "key-2"}
	v := ViewPeer(peer, object.PermissionSet{Observe: true, Action: true})
	if !v.HasPresharedKey {
		t.Errorf("HasPresharedKey must be true")
	}
	if v.PublicKeyDigest == peer.PublicKey {
		t.Errorf("public key leaked into view")
	}
	js, _ := json.Marshal(v)
	for _, leak := range []string{"vault-1", "key-2", peer.PublicKey} {
		if strings.Contains(string(js), leak) {
			t.Errorf("peer view leaks %q", leak)
		}
	}
}

func TestTorture_ViewSupportedActions(t *testing.T) {
	s := makeTestServer()
	v := ViewServer(s, object.PermissionSet{Observe: true})
	for _, a := range []string{"show", "edit", "delete", "rotate", "add-peer", "remove-peer"} {
		if !v.SupportedActions.Includes(a) {
			t.Errorf("server view must support %q", a)
		}
	}
	peer := s.Peers[0]
	pv := ViewPeer(peer, object.PermissionSet{Observe: true})
	for _, a := range []string{"show", "edit", "delete", "disable", "revoke", "rotate"} {
		if !pv.SupportedActions.Includes(a) {
			t.Errorf("peer view must support %q", a)
		}
	}
}

func TestTorture_ViewRevIncludesRevision(t *testing.T) {
	s := makeTestServer()
	v := ViewServer(s, object.PermissionSet{Observe: true})
	if v.Ref.Kind != "wireguard.server" {
		t.Errorf("unexpected kind: %q", v.Ref.Kind)
	}
	if v.Ref.ID != s.ID {
		t.Errorf("unexpected id: %q", v.Ref.ID)
	}
	// Revision must NOT leak the raw public key; it is the
	// short sha256 digest of the public key bytes.
	if v.Ref.Revision == s.PublicKey {
		t.Errorf("revision leaked raw public key: %q", v.Ref.Revision)
	}
	if v.Ref.Revision == "" {
		t.Errorf("revision must be populated for the operator to correlate rows")
	}
}
