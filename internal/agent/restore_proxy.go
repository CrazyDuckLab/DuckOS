package agent

import (
	"net/http"
)

// RestoreProxy possesses no filesystem write authority. Its configured local
// socket is not accepted from the public request.
type RestoreProxy struct{ transport *UpdaterProxy }

func NewRestoreProxy(socket string) *RestoreProxy { return &RestoreProxy{NewUpdaterProxy(socket)} }
func (p *RestoreProxy) ServeRestore(w http.ResponseWriter, r *http.Request) {
	operation := "restore"
	if r.URL.Path == BackupRestorePath+"/reconcile" {
		operation = "reconcile"
	}
	p.transport.forward(r.Context(), w, r, operation)
}
