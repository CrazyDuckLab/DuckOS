package brand

import (
	"os"
	"strings"
	"testing"
)

func TestMarkIsCompactASCIIAndInstallerMatches(t *testing.T) {
	lines := strings.Split(Mark, "\n")
	if len(lines) != 2 {
		t.Fatalf("mark must occupy two lines, got %d", len(lines))
	}
	// The current intentional canonical mark is two lines of
	// different widths:
	//   line 1: `<(o)__/`   (7 chars)
	//   line 2: ` (___/`    (6 chars, leading space aligns with
	//                        line 1 visually)
	// The earlier "2 × 6" contract is obsolete — the trailing
	// `/` on line 1 is part of the current mark. We validate
	// the asymmetric shape directly rather than a single uniform
	// width so the canonical form is described in code.
	wantWidths := []int{7, 6}
	wantFirst := "<(o)__/"
	wantSecond := " (___/"
	got := []string{lines[0], lines[1]}
	for i, line := range got {
		if len(line) != wantWidths[i] {
			t.Fatalf("mark line %d must be %d monospace columns, got %q (len=%d)",
				i+1, wantWidths[i], line, len(line))
		}
		for _, c := range line {
			if c < 32 || c > 126 {
				t.Fatalf("mark must use printable ASCII, got %q", line)
			}
		}
	}
	if got[0] != wantFirst || got[1] != wantSecond {
		t.Fatalf("canonical mark mismatch: got %q, want %q / %q", got, wantFirst, wantSecond)
	}
	script, err := os.ReadFile("../../scripts/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "'"+lines[0]+"' '"+lines[1]+"'") {
		t.Fatalf("installer and canonical mark differ: installer must contain '%s' '%s'", lines[0], lines[1])
	}
	// Note: web/public/duck.svg was intentionally replaced by
	// the rubber-duck SVG mark; the favicon is no longer an
	// ASCII-text rendition of pkg/brand/mark.txt. We do not
	// assert equality here — the SVG is its own artifact, the
	// canonical mark is the runtime installer line above.
}
