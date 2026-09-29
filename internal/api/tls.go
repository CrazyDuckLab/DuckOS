package api

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"os"
	"syscall"
	"time"
)

// LoadTLSConfig rejects unsafe key paths and unusable certificates before bind.
// Certificate trust is an administrator decision; this function does not bypass
// client verification or automatically expose the API on another interface.
func LoadTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	certPEM, err := readTLSFile(certFile, false)
	if err != nil {
		return nil, err
	}
	keyPEM, err := readTLSFile(keyFile, true)
	if err != nil {
		return nil, err
	}
	defer clear(keyPEM)
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, errors.New("TLS certificate and key are invalid or mismatched")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, errors.New("TLS certificate is invalid")
	}
	if time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
		return nil, errors.New("TLS certificate is expired or not yet valid")
	}
	if err = leaf.VerifyHostname("127.0.0.1"); err != nil {
		return nil, errors.New("loopback TLS certificate must include IP SAN 127.0.0.1")
	}
	cert.Leaf = leaf
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, NextProtos: []string{"http/1.1"}}, nil
}
func readTLSFile(path string, secret bool) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, errors.New("TLS file unavailable")
	}
	mask := os.FileMode(0022)
	if secret {
		mask = 0037
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&mask != 0 || info.Size() > 1<<20 {
		return nil, errors.New("TLS file has unsafe type, permissions or size")
	}
	if st, ok := info.Sys().(*syscall.Stat_t); !ok || st.Nlink != 1 || (st.Uid != 0 && int(st.Uid) != os.Geteuid()) {
		return nil, errors.New("TLS file has unsafe ownership or hard links")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("TLS file unavailable")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("TLS file changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errors.New("TLS file read failed or exceeded size bound")
	}
	return data, nil
}

// EnableTLS is startup-only and must accompany a TLS listener. It binds Origin
// validation to HTTPS; forwarded headers never select the trusted scheme.
func (h *Handler) EnableTLS() { h.tlsOnly = true }
