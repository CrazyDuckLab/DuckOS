// Helpers for the agent package tests.
package agent

import "os"

func readFileForTest(path string) ([]byte, error) { return os.ReadFile(path) }
