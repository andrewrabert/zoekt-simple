// Package metrics provides Prometheus metrics for zoekt-simple.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metrics for the application.
type Metrics struct {
	// HTTP handler metrics.
	HTTPRequestsTotal    *prometheus.CounterVec
	HTTPRequestDuration  *prometheus.HistogramVec
	HTTPResponseSize     *prometheus.HistogramVec
	HTTPRequestsInFlight prometheus.Gauge

	// Search-specific metrics.
	SearchRequestsTotal   *prometheus.CounterVec
	SearchRequestDuration *prometheus.HistogramVec

	// Index stats (set via gauge callbacks).
	IndexRepos     prometheus.Gauge
	IndexShards    prometheus.Gauge
	IndexDocuments prometheus.Gauge

	// Indexer metrics.
	IndexerRunsTotal    *prometheus.CounterVec
	IndexerRunDuration  *prometheus.HistogramVec
	IndexerLastRunTime  prometheus.Gauge
	IndexerReposIndexed prometheus.Counter
	IndexerErrors       prometheus.Counter

	// Index queue metrics.
	IndexQueueHighLength prometheus.GaugeFunc
	IndexQueueLowLength  prometheus.GaugeFunc

	registry *prometheus.Registry
}

// New creates a new Metrics instance with all metrics registered.
func New() *Metrics {
	reg := prometheus.NewRegistry()

	// Register standard Go runtime and process collectors.
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	m := &Metrics{
		registry: reg,

		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "zoekt",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests.",
		}, []string{"method", "path", "code"}),

		HTTPRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "zoekt",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "path"}),

		HTTPResponseSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "zoekt",
			Subsystem: "http",
			Name:      "response_size_bytes",
			Help:      "HTTP response size in bytes.",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 7), // 100B to 100MB
		}, []string{"method", "path"}),

		HTTPRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "zoekt",
			Subsystem: "http",
			Name:      "requests_in_flight",
			Help:      "Number of HTTP requests currently being served.",
		}),

		SearchRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "zoekt",
			Subsystem: "search",
			Name:      "requests_total",
			Help:      "Total number of search requests.",
		}, []string{"output_mode", "status"}),

		SearchRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "zoekt",
			Subsystem: "search",
			Name:      "request_duration_seconds",
			Help:      "Search request duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"output_mode"}),

		IndexRepos: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "zoekt",
			Subsystem: "index",
			Name:      "repos",
			Help:      "Number of indexed repositories.",
		}),

		IndexShards: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "zoekt",
			Subsystem: "index",
			Name:      "shards",
			Help:      "Number of index shards.",
		}),

		IndexDocuments: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "zoekt",
			Subsystem: "index",
			Name:      "documents",
			Help:      "Number of indexed documents.",
		}),

		IndexerRunsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "zoekt",
			Subsystem: "indexer",
			Name:      "runs_total",
			Help:      "Total number of indexer runs.",
		}, []string{"status"}),

		IndexerRunDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "zoekt",
			Subsystem: "indexer",
			Name:      "run_duration_seconds",
			Help:      "Duration of individual index operations in seconds.",
			Buckets:   prometheus.ExponentialBuckets(0.1, 2, 15), // 0.1s to ~1638s
		}, []string{"status"}),

		IndexerLastRunTime: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "zoekt",
			Subsystem: "indexer",
			Name:      "last_run_timestamp_seconds",
			Help:      "Unix timestamp of the last indexer run.",
		}),

		IndexerReposIndexed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "zoekt",
			Subsystem: "indexer",
			Name:      "repos_indexed_total",
			Help:      "Total number of repos indexed.",
		}),

		IndexerErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "zoekt",
			Subsystem: "indexer",
			Name:      "errors_total",
			Help:      "Total number of indexer errors.",
		}),
	}

	// Register all metrics.
	reg.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.HTTPResponseSize,
		m.HTTPRequestsInFlight,
		m.SearchRequestsTotal,
		m.SearchRequestDuration,
		m.IndexRepos,
		m.IndexShards,
		m.IndexDocuments,
		m.IndexerRunsTotal,
		m.IndexerRunDuration,
		m.IndexerLastRunTime,
		m.IndexerReposIndexed,
		m.IndexerErrors,
	)

	return m
}

// RegisterQueueMetrics registers index queue length gauges. This is separate
// from New because the queue is created after the metrics instance.
func (m *Metrics) RegisterQueueMetrics(highLen, lowLen func() float64) {
	m.IndexQueueHighLength = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "zoekt",
		Subsystem: "indexer",
		Name:      "queue_high_length",
		Help:      "Number of items in the high-priority index queue.",
	}, highLen)
	m.registry.MustRegister(m.IndexQueueHighLength)

	m.IndexQueueLowLength = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "zoekt",
		Subsystem: "indexer",
		Name:      "queue_low_length",
		Help:      "Number of items in the low-priority index queue.",
	}, lowLen)
	m.registry.MustRegister(m.IndexQueueLowLength)
}

// Handler returns the HTTP handler for the /metrics endpoint.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// ObserveSearch records metrics for a search request.
func (m *Metrics) ObserveSearch(outputMode string, duration time.Duration, err error) {
	status := "ok"
	if err != nil {
		status = "error"
	}
	m.SearchRequestsTotal.WithLabelValues(outputMode, status).Inc()
	m.SearchRequestDuration.WithLabelValues(outputMode).Observe(duration.Seconds())
}

// ObserveIndexRun records metrics for an index run.
func (m *Metrics) ObserveIndexRun(duration time.Duration, err error) {
	status := "ok"
	if err != nil {
		status = "error"
		m.IndexerErrors.Inc()
	}
	m.IndexerRunsTotal.WithLabelValues(status).Inc()
	m.IndexerRunDuration.WithLabelValues(status).Observe(duration.Seconds())
	m.IndexerLastRunTime.SetToCurrentTime()
	if err == nil {
		m.IndexerReposIndexed.Inc()
	}
}

// responseWriter wraps http.ResponseWriter to capture status code and bytes written.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

// Wrap returns middleware that instruments an http.Handler with request metrics.
// The pathLabel function maps the request to a label value for the path dimension
// (to avoid high-cardinality labels from path parameters).
func (m *Metrics) Wrap(pathLabel func(*http.Request) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.HTTPRequestsInFlight.Inc()
		defer m.HTTPRequestsInFlight.Dec()

		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		path := pathLabel(r)

		m.HTTPRequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(rw.statusCode)).Inc()
		m.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(duration.Seconds())
		m.HTTPResponseSize.WithLabelValues(r.Method, path).Observe(float64(rw.bytes))
	})
}
