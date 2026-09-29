// Package brand provides the same ASCII-only mark used by Web and terminals.
package brand

import (
	_ "embed"
	"strings"
)

//go:embed mark.txt
var source string

var Mark = strings.TrimSuffix(source, "\n")
