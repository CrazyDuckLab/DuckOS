package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

type agentLinuxSource struct {
	client *http.Client
}

func newAgentLinuxSource(socket string) *agentLinuxSource {
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", socket)
	}, MaxIdleConns: 2, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 2, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 4 * time.Second, MaxResponseHeaderBytes: 8192, DisableCompression: true}
	return &agentLinuxSource{client: &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (s *agentLinuxSource) FirewallRules(ctx context.Context) ([]object.ViewObject, error) {
	return s.fetch(ctx, "network.firewall.rule")
}
func (s *agentLinuxSource) FirewallSets(ctx context.Context) ([]object.ViewObject, error) {
	return s.fetch(ctx, "network.firewall.set")
}
func (s *agentLinuxSource) DHCPLeases(ctx context.Context) ([]object.ViewObject, error) {
	return s.fetch(ctx, "network.lease")
}
func (s *agentLinuxSource) DNSZones(ctx context.Context) ([]object.ViewObject, error) {
	return s.fetch(ctx, "dns.zone")
}
func (s *agentLinuxSource) DHCPSubnets(ctx context.Context) ([]object.ViewObject, error) {
	return s.fetch(ctx, "dhcp.subnet")
}
func (s *agentLinuxSource) AnycastBGPPeers(ctx context.Context) ([]object.ViewObject, error) {
	return s.fetch(ctx, "bgp.peer")
}
func (s *agentLinuxSource) AnycastOSPFAreas(ctx context.Context) ([]object.ViewObject, error) {
	return s.fetch(ctx, "ospf.area")
}
func (s *agentLinuxSource) AnycastISISAreas(ctx context.Context) ([]object.ViewObject, error) {
	return s.fetch(ctx, "isis.area")
}
func (s *agentLinuxSource) AnycastRouteMaps(ctx context.Context) ([]object.ViewObject, error) {
	return s.fetch(ctx, "route.map")
}

func (s *agentLinuxSource) fetch(ctx context.Context, kind string) ([]object.ViewObject, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("Agent Linux source is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent"+agent.LinuxObjectsPathPrefix+kind, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return nil, errors.New("Agent Linux source is unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 128<<10+1))
	if err != nil {
		return nil, errors.New("Agent Linux source response read failed")
	}
	if len(data) > 128<<10 {
		return nil, errors.New("Agent Linux source response exceeds size limit")
	}
	var col object.Collection
	if err := json.Unmarshal(data, &col); err != nil {
		return nil, errors.New("Agent Linux source response decode failed")
	}
	if !col.Complete {
		return nil, errors.New("Agent Linux source returned incomplete data")
	}
	return col.Items, nil
}
