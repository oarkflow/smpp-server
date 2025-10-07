package metrics

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusMetricsCollector implements MetricsCollector using Prometheus
type PrometheusMetricsCollector struct {
	registry *prometheus.Registry

	// Counters
	smsSubmittedTotal *prometheus.CounterVec
	smsDeliveredTotal *prometheus.CounterVec
	smsFailedTotal    *prometheus.CounterVec
	authFailuresTotal *prometheus.CounterVec
	bindSuccessTotal  *prometheus.CounterVec
	connectionTotal   *prometheus.CounterVec
	pduProcessedTotal *prometheus.CounterVec

	// Gauges
	activeConnections   *prometheus.GaugeVec
	rateLimitedRequests *prometheus.GaugeVec
	queueSize           *prometheus.GaugeVec

	// Histograms
	pduProcessingDuration *prometheus.HistogramVec
	messageDeliveryTime   *prometheus.HistogramVec
	connectionDuration    *prometheus.HistogramVec

	// Mutex for thread safety
	mu sync.RWMutex

	// HTTP server for metrics endpoint
	server *http.Server
}

// NewPrometheusMetricsCollector creates a new Prometheus metrics collector
func NewPrometheusMetricsCollector(port int) *PrometheusMetricsCollector {
	registry := prometheus.NewRegistry()

	pmc := &PrometheusMetricsCollector{
		registry: registry,
	}

	// Initialize counters
	pmc.smsSubmittedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smpp_sms_submitted_total",
			Help: "Total number of SMS messages submitted",
		},
		[]string{"source_ton", "dest_ton", "data_coding", "bind_type"},
	)

	pmc.smsDeliveredTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smpp_sms_delivered_total",
			Help: "Total number of SMS messages delivered",
		},
		[]string{"source_ton", "dest_ton", "data_coding"},
	)

	pmc.smsFailedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smpp_sms_failed_total",
			Help: "Total number of SMS messages that failed",
		},
		[]string{"reason", "bind_type"},
	)

	pmc.authFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smpp_auth_failures_total",
			Help: "Total number of authentication failures",
		},
		[]string{"reason"},
	)

	pmc.bindSuccessTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smpp_bind_success_total",
			Help: "Total number of successful binds",
		},
		[]string{"bind_type", "system_type"},
	)

	pmc.connectionTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smpp_connections_total",
			Help: "Total number of connections",
		},
		[]string{"type", "result"},
	)

	pmc.pduProcessedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smpp_pdu_processed_total",
			Help: "Total number of PDUs processed",
		},
		[]string{"command_id", "direction", "result"},
	)

	// Initialize gauges
	pmc.activeConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "smpp_active_connections",
			Help: "Number of active connections",
		},
		[]string{"bind_type"},
	)

	pmc.rateLimitedRequests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "smpp_rate_limited_requests",
			Help: "Number of rate limited requests",
		},
		[]string{"system_id"},
	)

	pmc.queueSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "smpp_queue_size",
			Help: "Size of message queues",
		},
		[]string{"queue_type"},
	)

	// Initialize histograms
	pmc.pduProcessingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "smpp_pdu_processing_duration_seconds",
			Help:    "Time spent processing PDUs",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"command_id", "direction"},
	)

	pmc.messageDeliveryTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "smpp_message_delivery_time_seconds",
			Help:    "Time taken to deliver messages",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300},
		},
		[]string{"priority"},
	)

	pmc.connectionDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "smpp_connection_duration_seconds",
			Help:    "Duration of connections",
			Buckets: []float64{1, 10, 60, 300, 900, 3600, 7200, 21600, 43200},
		},
		[]string{"bind_type"},
	)

	// Register all metrics
	registry.MustRegister(
		pmc.smsSubmittedTotal,
		pmc.smsDeliveredTotal,
		pmc.smsFailedTotal,
		pmc.authFailuresTotal,
		pmc.bindSuccessTotal,
		pmc.connectionTotal,
		pmc.pduProcessedTotal,
		pmc.activeConnections,
		pmc.rateLimitedRequests,
		pmc.queueSize,
		pmc.pduProcessingDuration,
		pmc.messageDeliveryTime,
		pmc.connectionDuration,
	)

	// Start HTTP server for metrics endpoint
	if port > 0 {
		pmc.startMetricsServer(port)
	}

	return pmc
}

// IncCounter increments a counter metric
func (p *PrometheusMetricsCollector) IncCounter(name string, labels map[string]string) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	switch name {
	case "sms_submitted_total":
		p.smsSubmittedTotal.With(labels).Inc()
	case "sms_delivered_total":
		p.smsDeliveredTotal.With(labels).Inc()
	case "sms_failed_total":
		p.smsFailedTotal.With(labels).Inc()
	case "auth_failures_total":
		p.authFailuresTotal.With(labels).Inc()
	case "bind_success_total":
		p.bindSuccessTotal.With(labels).Inc()
	case "connections_total":
		p.connectionTotal.With(labels).Inc()
	case "pdu_processed_total":
		p.pduProcessedTotal.With(labels).Inc()
	default:
		// Unknown counter, could log or ignore
	}
}

// SetGauge sets a gauge metric
func (p *PrometheusMetricsCollector) SetGauge(name string, value float64, labels map[string]string) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	switch name {
	case "active_connections":
		p.activeConnections.With(labels).Set(value)
	case "rate_limited_requests":
		p.rateLimitedRequests.With(labels).Set(value)
	case "queue_size":
		p.queueSize.With(labels).Set(value)
	default:
		// Unknown gauge, could log or ignore
	}
}

// ObserveHistogram observes a value for a histogram metric
func (p *PrometheusMetricsCollector) ObserveHistogram(name string, value float64, labels map[string]string) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	switch name {
	case "pdu_processing_duration":
		p.pduProcessingDuration.With(labels).Observe(value)
	case "message_delivery_time":
		p.messageDeliveryTime.With(labels).Observe(value)
	case "connection_duration":
		p.connectionDuration.With(labels).Observe(value)
	default:
		// Unknown histogram, could log or ignore
	}
}

// RecordDuration records a duration metric
func (p *PrometheusMetricsCollector) RecordDuration(name string, duration time.Duration, labels map[string]string) {
	p.ObserveHistogram(name+"_duration", duration.Seconds(), labels)
}

// startMetricsServer starts the HTTP server for Prometheus metrics
func (p *PrometheusMetricsCollector) startMetricsServer(port int) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{}))

	p.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error if logger is available
		}
	}()
}

// Stop stops the metrics collector and HTTP server
func (p *PrometheusMetricsCollector) Stop() error {
	if p.server != nil {
		return p.server.Close()
	}
	return nil
}

// NoOpMetricsCollector provides a no-op implementation for when metrics are disabled
type NoOpMetricsCollector struct{}

// NewNoOpMetricsCollector creates a no-op metrics collector
func NewNoOpMetricsCollector() *NoOpMetricsCollector {
	return &NoOpMetricsCollector{}
}

// IncCounter is a no-op
func (n *NoOpMetricsCollector) IncCounter(name string, labels map[string]string) {}

// SetGauge is a no-op
func (n *NoOpMetricsCollector) SetGauge(name string, value float64, labels map[string]string) {}

// ObserveHistogram is a no-op
func (n *NoOpMetricsCollector) ObserveHistogram(name string, value float64, labels map[string]string) {
}

// RecordDuration is a no-op
func (n *NoOpMetricsCollector) RecordDuration(name string, duration time.Duration, labels map[string]string) {
}
