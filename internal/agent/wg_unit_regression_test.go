// Regression test for B-009 (BETA.2): a previous version of
// packaging/systemd/duck-wg-launcher.service listed
// "Conflicts=wg-quick@.service wg-quick@*.service" (and
// later "wg-quick@*"). systemd rejects every variant of a
// template-name glob in Conflicts= with "Invalid argument"
// and in some lab installs that error caused the unit to be
// silently dropped from the multi-user.target Wants graph
// and the launcher never started on reboot.
//
// This test pins the unit-file contract at the Go level so a
// regression cannot reintroduce the parse-error dependency.

package agent

import (
	"regexp"
	"strings"
	"testing"
)

// readPackagedWGLauncherUnit returns the in-repo unit text.
// The test lives in package agent but reads from the
// repository root at packaging/systemd/duck-wg-launcher.service.
func readPackagedWGLauncherUnit(t *testing.T) string {
	t.Helper()
	paths := []string{
		"../../packaging/systemd/duck-wg-launcher.service",
		"packaging/systemd/duck-wg-launcher.service",
	}
	for _, p := range paths {
		if b, err := readFileForTest(p); err == nil {
			return string(b)
		}
	}
	t.Skip("packaging/systemd/duck-wg-launcher.service not present in test cwd; CI must run from repo root")
	return ""
}

// TestWGLauncherUnitHasNoTemplateGlobConflicts asserts the
// unit file does not use any of the template-name globs that
// systemd rejects in Conflicts= (B-009 root cause). The
// enforcement is `systemctl mask wg-quick@.service` in the
// installer, not a Conflicts= line in the unit.
func TestWGLauncherUnitHasNoTemplateGlobConflicts(t *testing.T) {
	unit := readPackagedWGLauncherUnit(t)
	if unit == "" {
		return
	}
	// The unit is INI; Conflicts= lines are in [Unit].
	inUnit := false
	for _, line := range strings.Split(unit, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inUnit = line == "[Unit]"
			continue
		}
		if !inUnit {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(line), "conflicts=") {
			continue
		}
		val := strings.TrimPrefix(line, line[:len("Conflicts=")])
		val = strings.TrimSpace(val)
		// A valid Conflicts= value is one or more unit
		// names (no template instance globs).
		for _, tok := range strings.Fields(val) {
			if strings.Contains(tok, "*") {
				t.Fatalf("duck-wg-launcher.service Conflicts= must not use template-name glob %q; use `systemctl mask wg-quick@.service` in install.sh instead", tok)
			}
		}
	}
}

// TestWGLauncherUnitAfterAgent pins the production
// After=duck-agent.service line so a regression cannot
// reintroduce the "launcher starts before the agent has
// bound its RuntimeDirectory" race that produced the B-009
// lab install path.
func TestWGLauncherUnitAfterAgent(t *testing.T) {
	unit := readPackagedWGLauncherUnit(t)
	if unit == "" {
		return
	}
	re := regexp.MustCompile(`(?m)^\s*After=.*duck-agent\.service`)
	if !re.MatchString(unit) {
		t.Fatalf("duck-wg-launcher.service must declare After=duck-agent.service")
	}
	// Wants= must include duck-agent.service so the
	// launcher is pulled into the multi-user.target
	// Wants graph at boot.
	reW := regexp.MustCompile(`(?m)^\s*Wants=.*duck-agent\.service`)
	if !reW.MatchString(unit) {
		t.Fatalf("duck-wg-launcher.service must declare Wants=duck-agent.service so multi-user.target pulls it in at boot")
	}
}
