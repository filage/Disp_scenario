package observability

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	HTTPRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "analyst_http_requests_total",
			Help: "HTTP requests processed by the API.",
		},
		[]string{"method", "route", "status"},
	)
	HTTPDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "analyst_http_request_duration_seconds",
			Help:    "HTTP request latency.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)
	AnalysisJobs = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "analyst_analysis_jobs_total",
			Help: "Analysis job outcomes.",
		},
		[]string{"status", "provider"},
	)
	AnalysisDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "analyst_analysis_job_duration_seconds",
			Help:    "End-to-end worker processing duration.",
			Buckets: []float64{1, 5, 15, 30, 60, 120, 300, 600, 1200, 2400},
		},
		[]string{"provider"},
	)
	DependencyOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "analyst_dependency_operations_total",
			Help: "External dependency operation outcomes.",
		},
		[]string{"dependency", "operation", "status"},
	)
	DependencyDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "analyst_dependency_operation_duration_seconds",
			Help:    "External dependency operation duration.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 15, 30, 60, 120},
		},
		[]string{"dependency", "operation"},
	)
	SQLQueries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "analyst_sql_queries_total",
			Help: "PostgreSQL query outcomes.",
		},
		[]string{"operation", "status"},
	)
	SQLDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "analyst_sql_query_duration_seconds",
			Help:    "PostgreSQL query duration.",
			Buckets: []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"operation"},
	)
	OutboxEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "analyst_outbox_events_total",
			Help: "Transactional outbox publication outcomes.",
		},
		[]string{"status"},
	)
	ReconciliationActions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "analyst_reconciliation_actions_total",
			Help: "Recovery actions performed by the reconciler.",
		},
		[]string{"action", "status"},
	)
	CleanupFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "analyst_cleanup_failures_total",
			Help: "Cleanup failures by component.",
		},
		[]string{"component"},
	)
)

func init() {
	prometheus.MustRegister(
		HTTPRequests,
		HTTPDuration,
		AnalysisJobs,
		AnalysisDuration,
		DependencyOperations,
		DependencyDuration,
		SQLQueries,
		SQLDuration,
		OutboxEvents,
		ReconciliationActions,
		CleanupFailures,
	)
	for _, status := range []string{"published", "failed"} {
		OutboxEvents.WithLabelValues(status).Add(0)
	}
	for _, action := range []string{"missing_publication", "worker_timeout", "expired_upload"} {
		for _, status := range []string{"recovered", "failed"} {
			ReconciliationActions.WithLabelValues(action, status).Add(0)
		}
	}
	for _, component := range []string{"pipeline_temp", "recording_objects", "expired_upload"} {
		CleanupFailures.WithLabelValues(component).Add(0)
	}
}

type correlationIDKey struct{}

func HTTPMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			wrapped := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			correlationID := r.Header.Get("X-Correlation-ID")
			if correlationID == "" {
				correlationID = chimiddleware.GetReqID(r.Context())
			}
			w.Header().Set("X-Correlation-ID", correlationID)
			r = r.WithContext(WithCorrelationID(r.Context(), correlationID))
			next.ServeHTTP(wrapped, r)
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}
			status := wrapped.Status()
			if status == 0 {
				status = http.StatusOK
			}
			elapsed := time.Since(started)
			HTTPRequests.WithLabelValues(r.Method, route, strconv.Itoa(status)).Inc()
			HTTPDuration.WithLabelValues(r.Method, route).Observe(elapsed.Seconds())
			logger.Info("request completed",
				"method", r.Method, "route", route, "status", status,
				"duration_ms", elapsed.Milliseconds(),
				"request_id", chimiddleware.GetReqID(r.Context()),
				"correlation_id", correlationID,
			)
		})
	}
}

func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationIDKey{}, strings.TrimSpace(correlationID))
}

func CorrelationID(ctx context.Context) string {
	value, _ := ctx.Value(correlationIDKey{}).(string)
	return value
}

func ObserveDependency(dependency, operation string, started time.Time, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}
	DependencyOperations.WithLabelValues(dependency, operation, status).Inc()
	DependencyDuration.WithLabelValues(dependency, operation).Observe(time.Since(started).Seconds())
}

type queryTraceKey struct{}

type queryTrace struct {
	started   time.Time
	operation string
}

type PGXTracer struct{}

func (PGXTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryTraceKey{}, queryTrace{
		started:   time.Now(),
		operation: sqlOperation(data.SQL),
	})
}

func (PGXTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	trace, ok := ctx.Value(queryTraceKey{}).(queryTrace)
	if !ok {
		return
	}
	status := "success"
	if data.Err != nil {
		status = "error"
	}
	SQLQueries.WithLabelValues(trace.operation, status).Inc()
	SQLDuration.WithLabelValues(trace.operation).Observe(time.Since(trace.started).Seconds())
}

func sqlOperation(sql string) string {
	fields := strings.Fields(strings.TrimSpace(sql))
	if len(fields) == 0 {
		return "unknown"
	}
	operation := strings.ToLower(fields[0])
	switch operation {
	case "select", "insert", "update", "delete", "with", "create", "alter", "drop":
		return operation
	default:
		return "other"
	}
}
