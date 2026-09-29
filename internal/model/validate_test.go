package model_test

import (
	"bytes"
	"encoding/json"
	"github.com/CrazyDuckLab/DuckOS/internal/model"
	"github.com/CrazyDuckLab/DuckOS/internal/testfixture"
	"strings"
	"testing"
	"time"
)

func TestDecodeOverviewRejectsInvalidBoundaryData(t *testing.T) {
	o := testfixture.Overview()
	valid, _ := json.Marshal(o)
	if _, e := model.DecodeOverview(valid, "test", time.Now()); e != nil {
		t.Fatal(e)
	}
	cases := map[string][]byte{"oversize": bytes.Repeat([]byte(" "), model.MaxResponseBytes+1), "unknown": append([]byte(`{"extra":1,`), valid[1:]...), "missing": bytes.Replace(valid, []byte(`"schemaVersion":"1",`), nil, 1), "null": bytes.Replace(valid, []byte(`"issues":[]`), []byte(`"issues":null`), 1), "trailing": append(append([]byte{}, valid...), []byte(`{}`)...), "schema": bytes.Replace(valid, []byte(`"schemaVersion":"1"`), []byte(`"schemaVersion":"2"`), 1)}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, e := model.DecodeOverview(data, "test", time.Now()); e == nil {
				t.Fatal("invalid IPC accepted")
			}
		})
	}
	if _, e := model.DecodeOverview(valid, "production", time.Now()); e == nil {
		t.Fatal("environment mismatch")
	}
	if _, e := model.DecodeOverview(valid, "test", time.Now().Add(time.Minute)); e == nil {
		t.Fatal("stale state accepted")
	}
	o.Interfaces[0].Addresses[0].PrefixLength = 33
	b, _ := json.Marshal(o)
	if _, e := model.DecodeOverview(b, "test", time.Now()); e == nil {
		t.Fatal("invalid IPv4 prefix")
	}
}
func FuzzOverviewDecoder(f *testing.F) {
	b, _ := json.Marshal(testfixture.Overview())
	f.Add(b)
	f.Add([]byte(`null`))
	f.Add([]byte(strings.Repeat("{", 100)))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > model.MaxResponseBytes+1 {
			return
		}
		_, _ = model.DecodeOverview(b, "test", time.Now())
	})
}
