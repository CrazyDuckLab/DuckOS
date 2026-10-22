package center

import (
	"encoding/hex"
	"errors"
	"strings"
)

var errBadListen = errors.New("expected host:port")

// splitListen splits a host:port listen string.
func splitListen(s string) (string, string, error) {
	i := strings.LastIndex(s, ":")
	if i < 1 || i == len(s)-1 {
		return "", "", errBadListen
	}
	return s[:i], s[i+1:], nil
}

// trimSecret normalises resolved credential bytes.
func trimSecret(b []byte) string {
	return strings.TrimSpace(string(b))
}

// hexOf renders resolved short-id bytes as lowercase hex.
func hexOf(b []byte) string {
	return hex.EncodeToString(b)
}
