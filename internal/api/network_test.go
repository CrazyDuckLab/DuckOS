package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

func TestNetworkPreviewUsesRealEngineAndRoleBoundary(t *testing.T) {
	// No Agent exists here: offline preview must not depend on fake host state.
	h := handler(t, &testEngine{})
	h.engine = engine.New("/nonexistent/synthetic-agent.sock", "TEST")
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := identity.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h.EnableIdentity(s)
	request := func(token, body, path, method, content, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", content)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	const path = "/api/v1/network/preview"
	const candidate = `{"version":1,"interfaces":[{"id":"inside","name":"testlan0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"]}]}`
	const simulatePath = "/api/v1/network/simulate"
	const question = `{"ingress":"inside","source":"198.51.100.20","destination":"203.0.113.20","protocol":"tcp","port":443,"connectionState":"new"}`
	const simulation = `{"candidate":` + candidate + `,"question":` + question + `}`
	if w := request(testToken, candidate, path, "POST", "application/json", ""); w.Code != 403 {
		t.Fatal("unenrolled bootstrap planned network", w.Code)
	}
	const password = "Synthetic preview test password 42!"
	if _, err = s.Enroll("owner", password); err != nil {
		t.Fatal(err)
	}
	owner, _, err := s.Login("owner", password)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []identity.Role{identity.Owner, identity.NetworkAdmin, identity.Helpdesk, identity.Reception, identity.ReadOnly} {
		token := owner
		if role != identity.Owner {
			name := strings.ToLower(string(role))
			if _, err = s.CreateUser(owner, name, password, role); err != nil {
				t.Fatal(err)
			}
			token, _, err = s.Login(name, password)
			if err != nil {
				t.Fatal(err)
			}
		}
		w := request(token, candidate, path, "POST", "application/json", "")
		sim := request(token, simulation, simulatePath, "POST", "application/json", "")
		if role != identity.Owner && role != identity.NetworkAdmin {
			if w.Code != 403 || sim.Code != 403 {
				t.Fatal("unauthorized role could plan network", role, w.Code)
			}
			continue
		}
		if sim.Code != 200 || sim.Header().Get("Cache-Control") != "no-store, private" || !strings.Contains(sim.Body.String(), `"basis":"candidate-only"`) || !strings.Contains(sim.Body.String(), `"reachability":"not-tested"`) || !strings.Contains(sim.Body.String(), `"managedDecision":"drop"`) {
			t.Fatal("simulation did not retain offline/RBAC boundary", sim.Code, sim.Body.String())
		}
		var preview struct {
			HostValidated  bool `json:"hostValidated"`
			ApplyAvailable bool `json:"applyAvailable"`
			Plan           struct {
				Networkd map[string]string `json:"networkd"`
			} `json:"plan"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &preview) != nil || preview.HostValidated || preview.ApplyAvailable || !strings.Contains(preview.Plan.Networkd["10-duckos-inside.network"], "Address=198.51.100.1/24") {
			t.Fatal("invalid actual preview", w.Code, w.Body.String())
		}
	}
	for _, tc := range []struct {
		token, body, path, method, content, origin string
		want                                       int
	}{
		{testToken, candidate, path, "POST", "application/json", "", 401},
		{owner, candidate, path, "GET", "application/json", "", 405},
		{owner, candidate, path + "?apply=true", "POST", "application/json", "", 400},
		{owner, candidate, path, "POST", "text/plain", "", 415},
		{owner, candidate, path, "POST", "application/json", "https://attacker.test", 403},
		{owner, `{"version":1,"command":"anything"}`, path, "POST", "application/json", "", 422},
		{owner, candidate + ` {}`, path, "POST", "application/json", "", 422},
		{owner, strings.Repeat("x", state.MaxConfigBytes+1), path, "POST", "application/json", "", 400},
		{owner, candidate, "/api/v1/network/apply", "POST", "application/json", "", 404},
		{testToken, simulation, simulatePath, "POST", "application/json", "", 401},
		{owner, simulation, simulatePath + "?source=198.51.100.20", "POST", "application/json", "", 400},
		{owner, simulation, simulatePath, "GET", "application/json", "", 405},
		{owner, simulation, simulatePath, "POST", "text/plain", "", 415},
		{owner, simulation, simulatePath, "POST", "application/json", "https://attacker.test", 403},
		{owner, simulation + ` {}`, simulatePath, "POST", "application/json", "", 422},
		{owner, strings.Replace(simulation, `"connectionState":"new"`, `"connectionState":"new","execute":true`, 1), simulatePath, "POST", "application/json", "", 422},
	} {
		if w := request(tc.token, tc.body, tc.path, tc.method, tc.content, tc.origin); w.Code != tc.want {
			t.Fatalf("%s %s: got %d want %d", tc.method, tc.path, w.Code, tc.want)
		}
	}
}
