package observe

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/model"
)

const maxFileBytes = 128 << 10
const maxCommandBytes = 1 << 20

type Collector struct{ Environment string }

func readBounded(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxFileBytes {
		return nil, errors.New("observation size limit")
	}
	return b, nil
}

type limitedOutput struct {
	data   bytes.Buffer
	limit  int
	cancel context.CancelFunc
}

func (w *limitedOutput) Write(p []byte) (int, error) {
	if len(p) > w.limit-w.data.Len() {
		w.cancel()
		return 0, errors.New("command output limit")
	}
	return w.data.Write(p)
}

// runFixed is private: executable, argument vectors and environment originate
// only in reviewed collectors. Neither HTTP contract accepts command input.
func runFixed(parent context.Context, path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 1500*time.Millisecond)
	defer cancel()
	out := &limitedOutput{limit: maxCommandBytes, cancel: cancel}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = []string{"LANG=C", "LC_ALL=C", "PATH=/usr/sbin:/usr/bin", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat"}
	cmd.Stdout = out
	// Do not retain or expose daemon error output or nft rule text.
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 100 * time.Millisecond
	err := cmd.Run()
	return out.data.Bytes(), err
}

func emptyOverview(environment string) model.Overview {
	return model.Overview{SchemaVersion: model.SchemaVersion, Environment: environment, ObservedAt: time.Now().UTC(), CPU: model.CPU{LoadAverage: []float64{}}, Storage: []model.Storage{}, Interfaces: []model.Interface{}, Routes: []model.Route{}, DNS: model.DNS{Nameservers: []string{}, SearchDomains: []string{}, Source: "/etc/resolv.conf"}, Services: []model.Service{}, NFTables: model.NFTables{Status: "unavailable"}, Issues: []model.Issue{}}
}
func issue(o *model.Overview, component, code, message string) {
	o.Issues = append(o.Issues, model.Issue{Component: component, Code: code, Message: message})
}
