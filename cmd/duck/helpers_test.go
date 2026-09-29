package main

import (
	"io"
	"os"
)

func captureOutput(fn func() error) (string, error) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := fn()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), err
}

func writeToken(path, value string) error {
	return os.WriteFile(path, []byte(value+"\n"), 0o600)
}

func chmod(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}
