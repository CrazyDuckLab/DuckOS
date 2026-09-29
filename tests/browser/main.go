// A deliberately synthetic TEST-only browser harness, excluded from all product
// build targets. Real Unix transport and product API/Engine/Agent handler; only
// the Linux collector is replaced. Never use this as Linux runtime evidence.
package main

import (
	"context"
	"flag"
	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/api"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/model"
	"github.com/CrazyDuckLab/DuckOS/internal/testfixture"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

type synthetic struct{}

func (synthetic) Overview(context.Context) (model.Overview, error) {
	o := testfixture.Overview()
	o.System.Hostname = "SYNTHETIC-TEST.example"
	o.Memory.TotalBytes = 8 << 30
	o.Memory.AvailableBytes = 5 << 30
	o.Memory.UsedBytes = 3 << 30
	o.Storage[0].TotalBytes = 64 << 30
	o.Storage[0].AvailableBytes = 40 << 30
	o.Storage[0].UsedBytes = 24 << 30
	o.Issues = []model.Issue{{Component: "test", Code: "synthetic_browser_fixture", Message: "Synthetic TEST fixture for browser verification. These are not Linux observations."}}
	return o, nil
}
func main() {
	web := flag.String("web-dir", "web/dist", "Compiled TEST UI assets")
	flag.Parse()
	dir, err := os.MkdirTemp("", "duckos-ui-test-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := dir + "/agent.sock"
	l, err := net.Listen("unix", socket)
	if err != nil {
		log.Fatal(err)
	}
	s := &http.Server{Handler: agent.NewHandler(synthetic{}), ReadHeaderTimeout: time.Second}
	go func() { _ = s.Serve(l) }()
	defer s.Close()
	h, err := api.New(engine.New(socket, "test"), "synthetic-browser-test-token-123456789", "127.0.0.1:8080", *web)
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()
	log.Println("SYNTHETIC TEST harness listening at 127.0.0.1:8080; not a product service")
	server := &http.Server{Addr: "127.0.0.1:8080", Handler: h, ReadHeaderTimeout: 3 * time.Second}
	log.Fatal(server.ListenAndServe())
}
