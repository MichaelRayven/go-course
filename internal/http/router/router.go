// Package router glues the per-route handlers into the single
// api.ServerInterface generated from OpenAPI and builds the chi router.
package router

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	api "github.com/MichaelRayven/go-course/internal/generated"
	"github.com/MichaelRayven/go-course/internal/http/health"
	"github.com/MichaelRayven/go-course/internal/http/respond"
	"github.com/MichaelRayven/go-course/internal/http/trip/create"
	"github.com/MichaelRayven/go-course/internal/http/trip/finish"
	"github.com/MichaelRayven/go-course/internal/http/trip/get"
)

type Handlers struct {
	Create *create.Handler
	Get    *get.Handler
	Finish *finish.Handler
	Health *health.Handler
}

// server forwards each generated method to its own handler package.
// Endpoints of later labs fall through to api.Unimplemented (501).
type server struct {
	api.Unimplemented
	h Handlers
}

var _ api.ServerInterface = (*server)(nil)

func (s *server) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	s.h.Create.CreateTrip(w, r, params)
}

func (s *server) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	s.h.Get.GetTrip(w, r, tripID)
}

func (s *server) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	s.h.Finish.FinishTrip(w, r, tripID)
}

func (s *server) Health(w http.ResponseWriter, r *http.Request) {
	s.h.Health.Health(w, r)
}

func (s *server) Ready(w http.ResponseWriter, r *http.Request) {
	s.h.Health.Ready(w, r)
}

func New(h Handlers, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	// TODO(T20): replace with Recover -> Tracing -> Logging -> Metrics.
	r.Use(middleware.Recoverer)

	return api.HandlerWithOptions(&server{h: h}, api.ChiServerOptions{
		BaseRouter: r,
		// Called by the generated wrappers when a path, query or header
		// parameter fails to parse, e.g. a tripId that is not a UUID.
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
			respond.Error(w, r, logger, &trip.ValidationError{
				Field:  "parameters",
				Reason: "request parameters are invalid",
			})
		},
	})
}
