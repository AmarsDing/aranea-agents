package app

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"arenea/backend/internal/kernel/contracts"
	"arenea/backend/internal/kernel/module"
	"arenea/backend/internal/middleware"
)

// drivingRegistry is the concrete implementation of module.DrivingRegistry
// supplied during stage 3 (RegisterDriving). It owns the /api/v1 chi mount
// point and the OpenAPI fragment table.
type drivingRegistry struct {
	apiV1 chi.Router

	mu    sync.Mutex
	specs map[string]json.RawMessage
}

func newDrivingRegistry(apiV1 chi.Router) *drivingRegistry {
	return &drivingRegistry{
		apiV1: apiV1,
		specs: make(map[string]json.RawMessage),
	}
}

// WithAPIV1 invokes fn with the global /api/v1 chi.Router. Contexts use this
// hook to mount their handler trees.
func (d *drivingRegistry) WithAPIV1(fn func(r chi.Router)) {
	fn(d.apiV1)
}

// RegisterOpenAPISpec stores a Context's OpenAPI fragment under name. The
// last write wins; callers (typically Module.Name()) MUST use stable
// identifiers.
func (d *drivingRegistry) RegisterOpenAPISpec(name string, spec json.RawMessage) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.specs[name] = spec
}

// NewRouter assembles the global HTTP handler. It mounts /api/v1 once,
// invokes RegisterDriving on every module (stage 3), then wires the OpenAPI
// merge endpoint.
//
// Skeleton state (P0): the OpenAPI endpoint returns a constant placeholder.
// Real merging logic ships with the OpenAPI aggregation row (see §6.1 of
// the design document).
func NewRouter(modules []module.Module, reg *contracts.Registry) http.Handler {
	r := chi.NewRouter()
	apiV1 := chi.NewRouter()
	r.Mount("/api/v1", apiV1)

	d := newDrivingRegistry(apiV1)
	for _, m := range modules {
		m.RegisterDriving(d)
	}

	r.Get("/openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		merged, err := MergeOpenAPISpecs(modules)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(merged)
	})

	return r
}

// StackTransportMiddleware wraps the legacy transport HTTPHandler with the global
// middleware chain: CORS → RequestID → AccessLog → BasicAuth → RateLimit(60).
func StackTransportMiddleware(handler http.Handler) http.Handler {
	return middleware.RateLimit(60)(
		middleware.BasicAuth(
			middleware.AccessLog(
				middleware.RequestID(
					middleware.CORS(handler),
				),
			),
		),
	)
}
