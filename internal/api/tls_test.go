package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func testCertificate(t *testing.T, expired bool) (string, string, []byte) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Hour)
	if expired {
		until = time.Now().Add(-time.Minute)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic DuckOS TLS test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: until, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, DNSNames: []string{"localhost"}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	dir := t.TempDir()
	certPath, keyPath := dir+"/cert.pem", dir+"/key.pem"
	if err = os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath, certPEM
}
func TestTLS13AndOriginBoundary(t *testing.T) {
	cert, key, certPEM := testCertificate(t, false)
	config, err := LoadTLSConfig(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	h := handler(t, &testEngine{})
	h.EnableTLS()
	server := httptest.NewUnstartedServer(h)
	_, h.port, _ = net.SplitHostPort(server.Listener.Addr().String())
	server.TLS = config
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("certificate parse failed")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	for _, tc := range []struct {
		origin string
		status int
	}{{server.URL, 200}, {strings.Replace(server.URL, "https:", "http:", 1), 403}} {
		r, _ := http.NewRequest("GET", server.URL+"/api/v1/health", nil)
		r.Header.Set("Origin", tc.origin)
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tc.status || response.TLS.Version != tls.VersionTLS13 {
			t.Fatal("TLS origin boundary failed")
		}
	}
	old := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12}}
	defer old.CloseIdleConnections()
	legacy := &http.Client{Transport: old, Timeout: 3 * time.Second}
	if response, err := legacy.Get(server.URL + "/api/v1/health"); err == nil {
		response.Body.Close()
		t.Fatal("TLS 1.2 unexpectedly accepted")
	}
}
func TestTLSRejectsUnsafeExpiredAndMismatchedInputs(t *testing.T) {
	cert, key, _ := testCertificate(t, false)
	if err := os.Chmod(key, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTLSConfig(cert, key); err == nil {
		t.Fatal("public private key accepted")
	}
	if err := os.Chmod(key, 0600); err != nil {
		t.Fatal(err)
	}
	link := t.TempDir() + "/key-link"
	if err := os.Symlink(key, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTLSConfig(cert, link); err == nil {
		t.Fatal("symlink key accepted")
	}
	other, _, _ := testCertificate(t, false)
	if _, err := LoadTLSConfig(other, key); err == nil {
		t.Fatal("mismatched pair accepted")
	}
	expired, expiredKey, _ := testCertificate(t, true)
	if _, err := LoadTLSConfig(expired, expiredKey); err == nil {
		t.Fatal("expired certificate accepted")
	}
	h := handler(t, &testEngine{})
	h.EnableTLS()
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/v1/health", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("forwarded header bypassed TLS")
	}
}
