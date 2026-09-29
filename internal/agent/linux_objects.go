package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

const LinuxObjectsPathPrefix = "/agent/v1/objects/linux/"

const linuxObjectsMaxBytes = 128 << 10

var linuxObjectKinds = map[string]bool{
	"network.firewall.rule": true,
	"network.firewall.set":  true,
	"network.lease":         true,
	"dns.zone":              true,
	"dhcp.subnet":           true,
	"bgp.peer":              true,
	"ospf.area":             true,
	"isis.area":             true,
	"route.map":             true,
}

type linuxObjectRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	ReadFile(path string) ([]byte, error)
	Glob(pattern string) ([]string, error)
}

type osLinuxObjectRunner struct{}

func (osLinuxObjectRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}
func (osLinuxObjectRunner) ReadFile(path string) ([]byte, error)  { return os.ReadFile(path) }
func (osLinuxObjectRunner) Glob(pattern string) ([]string, error) { return filepath.Glob(pattern) }

type LinuxObjectsHost struct {
	runner linuxObjectRunner
}

func NewLinuxObjectsHost(r linuxObjectRunner) *LinuxObjectsHost {
	if r == nil {
		r = osLinuxObjectRunner{}
	}
	return &LinuxObjectsHost{runner: r}
}

func (h *LinuxObjectsHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.runner == nil {
		writeLinuxObjectsProblem(w, 503, "engine_unavailable", "Linux object source is not configured.")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeLinuxObjectsProblem(w, 405, "method_not_allowed", "Linux object source accepts GET only.")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		writeLinuxObjectsProblem(w, 400, "invalid_request", "Linux object request must not include body or query.")
		return
	}
	kind := strings.TrimPrefix(r.URL.EscapedPath(), LinuxObjectsPathPrefix)
	if !linuxObjectKinds[kind] {
		writeLinuxObjectsProblem(w, 404, "not_found", "Unknown Linux object kind.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2500*time.Millisecond)
	defer cancel()
	items, err := h.list(ctx, kind)
	if err != nil {
		writeLinuxObjectsProblem(w, 503, "engine_unavailable", err.Error())
		return
	}
	total := len(items)
	col := object.Collection{
		SnapshotID: "agent-linux-" + kind + "-" + shortHashObjects(items),
		Total:      &total,
		Complete:   true,
		Items:      items,
	}
	writeLinuxObjectsJSON(w, 200, col)
}

func (h *LinuxObjectsHost) list(ctx context.Context, kind string) ([]object.ViewObject, error) {
	switch kind {
	case "network.firewall.rule":
		return h.nftObjects(ctx, kind, "rule")
	case "network.firewall.set":
		return h.nftObjects(ctx, kind, "set")
	case "network.lease":
		return h.leaseObjects()
	case "dns.zone":
		return h.dnsmasqObjects(kind, "server=", "address=", "local=")
	case "dhcp.subnet":
		return h.dnsmasqObjects(kind, "dhcp-range=")
	case "bgp.peer", "ospf.area", "isis.area", "route.map":
		return h.frrObjects(ctx, kind)
	default:
		return nil, errors.New("unsupported Linux object kind")
	}
}

func (h *LinuxObjectsHost) nftObjects(ctx context.Context, kind, needle string) ([]object.ViewObject, error) {
	data, err := h.runner.Run(ctx, "nft", "-j", "list", "ruleset")
	if err != nil {
		return nil, errors.New("nftables runtime is unavailable")
	}
	if len(data) > linuxObjectsMaxBytes {
		return nil, errors.New("nftables observation exceeds size limit")
	}
	var decoded struct {
		NFTables []map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.NFTables == nil {
		return nil, errors.New("nftables observation is invalid")
	}
	rows := make([]object.ViewObject, 0)
	for _, entry := range decoded.NFTables {
		raw, ok := entry[needle]
		if !ok {
			continue
		}
		var observed map[string]any
		if err := json.Unmarshal(raw, &observed); err != nil || observed == nil {
			return nil, errors.New("nftables object is invalid")
		}
		rows = append(rows, linuxObservedObject(kind, kind+"-"+shortHashAny(observed), observed))
	}
	return rows, nil
}

func collectNFTObjects(kind string, v any, needle string) []object.ViewObject {
	var out []object.ViewObject
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case []any:
			for _, v := range t {
				walk(v)
			}
		case map[string]any:
			if raw, ok := t[needle]; ok {
				out = append(out, linuxObservedObject(kind, kind+"-"+shortHashAny(raw), raw))
			}
			for _, v := range t {
				walk(v)
			}
		}
	}
	walk(v)
	return out
}

func (h *LinuxObjectsHost) leaseObjects() ([]object.ViewObject, error) {
	paths := []string{"/var/lib/misc/dnsmasq.leases", "/var/lib/dnsmasq/dnsmasq.leases", "/var/lib/NetworkManager/dnsmasq.leases"}
	var found bool
	var out []object.ViewObject
	for _, p := range paths {
		data, err := h.runner.ReadFile(p)
		if err != nil {
			continue
		}
		found = true
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			id := fields[2] + "-" + fields[1]
			out = append(out, linuxObservedObject("network.lease", id, map[string]any{
				"expires": fields[0],
				"mac":     fields[1],
				"address": fields[2],
				"name":    fieldOr(fields, 3),
				"source":  p,
			}))
		}
	}
	if !found {
		return nil, errors.New("DHCP lease source is unavailable")
	}
	return out, nil
}

func (h *LinuxObjectsHost) dnsmasqObjects(kind string, prefixes ...string) ([]object.ViewObject, error) {
	files, _ := h.runner.Glob("/etc/dnsmasq.d/*.conf")
	files = append(files, "/etc/dnsmasq.conf")
	var found bool
	var out []object.ViewObject
	for _, p := range files {
		data, err := h.runner.ReadFile(p)
		if err != nil {
			continue
		}
		found = true
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			for _, pref := range prefixes {
				if strings.HasPrefix(line, pref) {
					out = append(out, linuxObservedObject(kind, shortHash(line), map[string]any{"line": scrubConfigLine(line), "source": p}))
					break
				}
			}
		}
	}
	if !found {
		return nil, errors.New("DNS/DHCP configuration source is unavailable")
	}
	return out, nil
}

func (h *LinuxObjectsHost) frrObjects(ctx context.Context, kind string) ([]object.ViewObject, error) {
	data, err := h.runner.Run(ctx, "vtysh", "-c", "show running-config")
	if err != nil {
		return nil, errors.New("FRR runtime observation is unavailable")
	}
	if len(data) > linuxObjectsMaxBytes {
		return nil, errors.New("FRR runtime observation exceeds size limit")
	}
	lines := relevantFRRLines(kind, string(data))
	out := make([]object.ViewObject, 0, len(lines))
	for _, line := range lines {
		out = append(out, linuxObservedObject(kind, shortHash(line), map[string]any{"line": scrubConfigLine(line), "source": "frr"}))
	}
	return out, nil
}

func relevantFRRLines(kind, data string) []string {
	var out []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case kind == "bgp.peer" && (strings.HasPrefix(line, "neighbor ") || strings.HasPrefix(line, "router bgp ")):
			out = append(out, line)
		case kind == "ospf.area" && strings.Contains(line, " area "):
			out = append(out, line)
		case kind == "isis.area" && (strings.HasPrefix(line, "router isis") || strings.HasPrefix(line, "net ")):
			out = append(out, line)
		case kind == "route.map" && strings.HasPrefix(line, "route-map "):
			out = append(out, line)
		}
	}
	return out
}

func linuxObservedObject(kind, id string, observed any) object.ViewObject {
	return object.ViewObject{
		Ref:              object.ObjectRef{Kind: kind, ID: id},
		DisplayName:      id,
		Observed:         observed,
		Availability:     object.Available,
		SupportedActions: object.SupportedActions{"observe"},
		Permissions:      object.PermissionSet{Observe: true},
	}
}

func writeLinuxObjectsJSON(w http.ResponseWriter, status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		writeLinuxObjectsProblem(w, 503, "encode_failed", "Linux object response could not be encoded.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeLinuxObjectsProblem(w http.ResponseWriter, status int, code, message string) {
	writeLinuxObjectsJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func scrubConfigLine(s string) string {
	lower := strings.ToLower(s)
	for _, word := range []string{"password", "secret", "key", "token"} {
		if strings.Contains(lower, word) {
			return "[redacted]"
		}
	}
	return s
}

func shortHashObjects(v []object.ViewObject) string { return shortHashAny(v) }
func shortHashAny(v any) string {
	data, _ := json.Marshal(v)
	return shortHash(string(data))
}
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}
func fieldOr(fields []string, i int) string {
	if i >= 0 && i < len(fields) {
		return fields[i]
	}
	return ""
}
