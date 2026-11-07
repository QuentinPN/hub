package api

import (
	"context"
	"net/http"
	"time"
)

// ContainerReader is implemented by stores that know which containers make a service.
type ContainerReader interface {
	// ServiceContainers returns ErrNotFound for an unknown, archived or example service.
	ServiceContainers(ctx context.Context, slug string, now time.Time) (ServiceContainers, error)
}

// registerContainers only reads: Hub never starts, stops or restarts a container.
func registerContainers(mux *http.ServeMux, reader ContainerReader) {
	mux.HandleFunc("GET /api/v1/services/{slug}/containers", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(reader.ServiceContainers(r.Context(), r.PathValue("slug"), time.Now()))
	})
}
