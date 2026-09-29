package main

import (
	"context"
	"net/http"
	"testing"
)

type verificationTransport func(*http.Request) (*http.Response, error)

func (f verificationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVerificationAgentPathNeverUsesPublicHTTP(t *testing.T) {
	called := false
	s := &cliState{endpoint: "http://127.0.0.1:8080", client: &http.Client{Transport: verificationTransport(func(*http.Request) (*http.Response, error) {
		called = true
		t.Fatal("private Agent path reached public HTTP")
		return nil, nil
	})}}
	if _, err := s.call(context.Background(), apiCall{method: "POST", path: "/agent/v1/network/apply"}, nil); err == nil {
		t.Fatal("missing socket accepted")
	}
	if called {
		t.Fatal("public transport used")
	}
	c := newAgentClient("/not-an-agent.sock")
	if c.CheckRedirect == nil || c.CheckRedirect(&http.Request{}, nil) != http.ErrUseLastResponse {
		t.Fatal("IPC client follows redirects")
	}
}
