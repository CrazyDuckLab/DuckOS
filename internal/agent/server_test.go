package agent

import (
	"context"
	"github.com/CrazyDuckLab/DuckOS/internal/model"
	"github.com/CrazyDuckLab/DuckOS/internal/testfixture"
	"net/http/httptest"
	"strings"
	"testing"
)

type fixtureObserver struct{ calls int }

func (o *fixtureObserver) Overview(context.Context) (model.Overview, error) {
	o.calls++
	return testfixture.Overview(), nil
}
func TestOnlyVersionedReadOperationAllowed(t *testing.T) {
	o := &fixtureObserver{}
	h := NewHandler(o)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{{"GET", OverviewPath, "", 200}, {"POST", OverviewPath, "", 405}, {"GET", "/agent/v2/overview", "", 404}, {"GET", OverviewPath + "?exec=anything", "", 400}, {"GET", OverviewPath, "anything", 400}, {"GET", "/agent/v1/execute", "", 404}} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s %s got%d", tc.method, tc.path, w.Code)
		}
	}
	if o.calls != 1 {
		t.Fatal("invalid request reached observer")
	}
}
func TestAgentConcurrencyBound(t *testing.T) {
	h := NewHandler(&fixtureObserver{})
	h.slots <- struct{}{}
	h.slots <- struct{}{}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", OverviewPath, nil))
	if w.Code != 503 {
		t.Fatal("unbounded parallel observation")
	}
}
