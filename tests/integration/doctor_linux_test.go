//go:build linux

package integration

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/observe"
)

// TestRealLinuxDoctorOverviewEndToEnd exercises the full
// observe+doctor pipeline against the actual Debian host:
// the Agent serves the live /agent/v1/overview over its
// loopback boundary and the doctor host collects real
// /proc, /etc/os-release, /proc/net/dev, /proc/net/route,
// /sys/class/net and /proc/loadavg data.
//
// The test never touches the management interface eth0.
func TestRealLinuxDoctorOverviewEndToEnd(t *testing.T) {
	socket := privateTempDir(t) + "/qualify-doctor.sock"
	_ = os.Remove(socket)
	l, err := agent.Listen(socket, uint32(os.Getuid()))
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: agent.NewHandler(&observe.Collector{Environment: "test"})}
	h := server.Handler.(*agent.Handler)
	h.EnableDoctor(agent.NewDoctorHost("test", "boot-1"))
	go func() { _ = server.Serve(l) }()
	defer server.Close()
	time.Sleep(200 * time.Millisecond)
	fetch := func(path string) []byte {
		client := &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socket)
				},
				DisableCompression: true,
			},
			Timeout: 5 * time.Second,
		}
		req, _ := http.NewRequest("GET", "http://agent"+path, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		buf := make([]byte, 0, 64*1024)
		chunk := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(chunk)
			if n > 0 {
				buf = append(buf, chunk[:n]...)
			}
			if err != nil {
				break
			}
		}
		return buf
	}
	overview := fetch("/agent/v1/overview")
	var o map[string]any
	if err := json.Unmarshal(overview, &o); err != nil {
		t.Fatal(err)
	}
	sys := o["system"].(map[string]any)
	if sys["hostname"].(string) == "" {
		t.Fatal("empty hostname")
	}
	if sys["kernel"].(string) == "" {
		t.Fatal("empty kernel")
	}
	if sys["uptimeSeconds"].(float64) <= 0 {
		t.Fatal("non-positive uptime")
	}
	if len(o["interfaces"].([]any)) == 0 {
		t.Fatal("no interfaces")
	}
	if o["memory"].(map[string]any)["totalBytes"].(float64) == 0 {
		t.Fatal("zero memory")
	}
	t.Logf("OVERVIEW hostname=%s kernel=%s uptime=%.0fs interfaces=%d memory_total=%d",
		sys["hostname"], sys["kernel"], sys["uptimeSeconds"],
		len(o["interfaces"].([]any)),
		int64(o["memory"].(map[string]any)["totalBytes"].(float64)))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	host := agent.NewDoctorHost("test", "boot-1")
	snap, err := host.Doctor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.System.Kernel == "" {
		t.Fatal("empty doctor kernel")
	}
	if snap.System.OS == "" {
		t.Fatal("empty doctor os")
	}
	if snap.System.Hostname == "" {
		t.Fatal("empty doctor hostname")
	}
	if snap.Resources.Memory.TotalBytes == 0 {
		t.Fatal("zero doctor memory")
	}
	if snap.Resources.CPULogicalCores == 0 {
		t.Fatal("zero doctor cpu cores")
	}
	t.Logf("DOCTOR schema=%s kernel=%q os=%q arch=%q cores=%d mem=%d storage=%d interfaces=%d routes=%d subsystems=%d",
		snap.SchemaVersion, snap.System.Kernel, snap.System.OS, snap.System.Architecture,
		snap.Resources.CPULogicalCores, snap.Resources.Memory.TotalBytes,
		len(snap.Resources.Storage), len(snap.Network.Interfaces), len(snap.Network.Routes), len(snap.Subsystem))
	for _, s := range snap.Subsystem {
		t.Logf("  subsystem %s=%s detail=%q", s.Name, s.Status, s.Detail)
	}
	if err := snap.Valid(); err != nil {
		t.Fatalf("doctor snapshot invalid: %v", err)
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
