// Package buildinfo carries immutable values supplied by the release build.
package buildinfo

var Version = "internal"
var Commit = "unknown"

func String() string { return "DuckOS " + Version + " (" + Commit + ")" }
