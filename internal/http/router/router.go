package router

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	api "github.com/MichaelRayven/go-course/internal/generated"
	"github.com/MichaelRayven/go-course/internal/http/health"
	"github.com/MichaelRayven/go-course/internal/http/middleware"
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

type Observability struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	Propagator     propagation.TextMapPropagator
}

func New(h Handlers, obs Observability, logger *slog.Logger) (http.Handler, error) {
	r := chi.NewRouter()

	route := func(req *http.Request) string {
		return r.Find(chi.NewRouteContext(), req.Method, req.URL.Path)
	}

	metrics, err := middleware.Metrics(obs.MeterProvider, route)
	if err != nil {
		return nil, err
	}

	r.Use(
		middleware.Recover(logger),
		middleware.Tracing(obs.TracerProvider, obs.Propagator, route),
		middleware.Logging(logger, route),
		metrics,
	)

	return api.HandlerWithOptions(&server{h: h}, api.ChiServerOptions{
		BaseRouter: r,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
			respond.Error(w, r, logger, &trip.ValidationError{
				Field:  "parameters",
				Reason: "request parameters are invalid",
			})
		},
	}), nil
}
