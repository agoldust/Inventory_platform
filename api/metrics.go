package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics registered on the default Prometheus registry and exposed at GET /metrics.
// The route label is the mux pattern (e.g. "GET /products/{id}"), not the raw
// path, so label cardinality stays bounded no matter which ids are requested.
var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests handled, by route pattern, status code and A/B variant.",
	}, []string{"route", "status", "variant"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency in seconds, by route pattern and A/B variant.",
		Buckets: prometheus.DefBuckets,
	}, []string{"route", "variant"})

	stockChanges = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stock_changes_total",
		Help: "Successful stock adjustments, by direction (increase or decrease).",
	}, []string{"direction"})
)

// statusRecorder remembers the status code a handler wrote so the middleware
// can label the request counter with it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader records code and passes it on to the wrapped ResponseWriter.
func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// instrument wraps next and records request count and latency for every
// request, labelled with the caller's A/B variant ("A", "B" or "none").
// Requests that match no route are labelled "unmatched".
func instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		// ServeMux sets r.Pattern on this same request while routing it.
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		variant := variantOf(r)
		httpRequests.WithLabelValues(route, strconv.Itoa(rec.status), variant).Inc()
		httpDuration.WithLabelValues(route, variant).Observe(time.Since(start).Seconds())
	})
}
