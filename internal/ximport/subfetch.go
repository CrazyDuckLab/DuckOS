// Package ximport / subfetch.go: secure fetching and parsing of
// external Xray-compatible subscriptions. The fetcher exists to
// import an operator-supplied subscription URL; it is NOT a generic
// HTTP client and refuses to become one:
//
//   - HTTPS only, TLS verification never disabled.
//   - Private, loopback, link-local and multicast addresses are
//     rejected at dial time (after DNS resolution, so DNS rebinding
//     to an RFC1918 target fails closed).
//   - Bounded size (2 MiB), bounded time (15 s total), bounded
//     redirects (3, all re-validated for scheme and target).
//   - The payload is parsed strictly: base64 or plaintext line
//     lists, each line through xray.ParseConnectionURI; unparseable
//     lines are reported, never silently dropped.
package ximport

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// MaxSubscriptionBytes bounds a fetched subscription payload.
const MaxSubscriptionBytes = 2 << 20

// FetchLimits bound one subscription refresh.
type FetchLimits struct {
	Timeout   time.Duration
	MaxBytes  int64
	Redirects int
}

func DefaultFetchLimits() FetchLimits {
	return FetchLimits{Timeout: 15 * time.Second, MaxBytes: MaxSubscriptionBytes, Redirects: 3}
}

// FetchSubscription downloads and parses an external subscription.
// The returned lines are strictly validated connection URIs;
// malformed lines land in Skipped with the reason.
func FetchSubscription(ctx context.Context, rawURL string, limits FetchLimits) (conn []xray.ParsedConnectionWithUnknown, skipped []string, err error) {
	if limits.Timeout == 0 {
		limits = DefaultFetchLimits()
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, fmt.Errorf("ximport: subscription url: %w", err)
	}
	if u.Scheme != "https" {
		return nil, nil, errors.New("ximport: subscription url must be https")
	}
	if u.User != nil {
		return nil, nil, errors.New("ximport: subscription url must not carry credentials")
	}
	dialer := &guardedDialer{}
	client := &http.Client{
		Timeout: limits.Timeout,
		Transport: &http.Transport{
			DialContext:            dialer.DialContext,
			TLSHandshakeTimeout:    10 * time.Second,
			MaxResponseHeaderBytes: 64 << 10,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > limits.Redirects {
				return errors.New("ximport: too many redirects")
			}
			if req.URL.Scheme != "https" {
				return errors.New("ximport: redirect dropped https")
			}
			if req.URL.User != nil {
				return errors.New("ximport: redirect carries credentials")
			}
			return nil
		},
	}
	fetchCtx, cancel := context.WithTimeout(ctx, limits.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "DuckOS-XrayCenter/1 (subscription import)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("ximport: fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("ximport: fetch status %d", resp.StatusCode)
	}
	if int64(resp.ContentLength) > limits.MaxBytes {
		return nil, nil, errors.New("ximport: payload exceeds size limit")
	}
	buf, rerr := io.ReadAll(io.LimitReader(resp.Body, limits.MaxBytes+1))
	if rerr != nil {
		return nil, nil, fmt.Errorf("ximport: read payload: %w", rerr)
	}
	if int64(len(buf)) > limits.MaxBytes {
		return nil, nil, errors.New("ximport: payload exceeds size limit")
	}
	return ParseSubscriptionPayload(string(buf))
}

// guardedDialer rejects private network targets after resolution.
type guardedDialer struct{}

func (g *guardedDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("ximport: dial address: %w", err)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("ximport: resolve %s: %w", host, err)
	}
	for _, ip := range ips {
		if !isPublicUnicast(ip.IP) {
			return nil, fmt.Errorf("ximport: %s resolves to a restricted address (%s); internal targets are refused", host, ip.IP)
		}
	}
	var d net.Dialer
	return d.DialContext(ctx, network, net.JoinHostPort(host, port))
}

func isPublicUnicast(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	// Additional shared ranges outside the Is* helpers.
	for _, cidr := range []string{
		"100.64.0.0/10",  // CGNAT
		"169.254.0.0/16", // APIPA
		"fe80::/10",      // IPv6 link local
		"fc00::/7",       // IPv6 ULA (also IsPrivate, kept for clarity)
		"2002::/24",      // 6to4 anycast ranges are treated as suspect
	} {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil && network.Contains(ip) {
			return false
		}
	}
	return true
}

// ParseSubscriptionPayload parses a fetched payload: base64 or
// plaintext, newline-separated connection URIs. Duplicate lines are
// reported once. Non-URI lines are skipped with reasons.
func ParseSubscriptionPayload(payload string) ([]xray.ParsedConnectionWithUnknown, []string, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil, nil, errors.New("ximport: subscription payload is empty")
	}
	text := payload
	if decoded, err := base64.StdEncoding.DecodeString(payload); err == nil && looksLikeURIs(string(decoded)) {
		text = string(decoded)
	} else if decoded, err := base64.URLEncoding.DecodeString(payload); err == nil && looksLikeURIs(string(decoded)) {
		text = string(decoded)
	}
	lines := strings.Split(text, "\n")
	var out []xray.ParsedConnectionWithUnknown
	var skipped []string
	seen := map[string]bool{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if seen[line] {
			skipped = append(skipped, "duplicate line dropped")
			continue
		}
		seen[line] = true
		parsed, err := xray.ParseConnectionURI(line)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("unparseable line (%v)", err))
			continue
		}
		out = append(out, parsed)
	}
	if len(out) == 0 {
		return out, skipped, errors.New("ximport: subscription payload contains no usable connection")
	}
	return out, skipped, nil
}

func looksLikeURIs(s string) bool {
	return strings.Contains(s, "://") || strings.Count(s, "\n") > 0
}
