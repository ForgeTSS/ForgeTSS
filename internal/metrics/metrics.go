// Package metrics provides Prometheus metrics for ForgeTSS.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var registry *prometheus.Registry

// Register initializes the global Prometheus registry and creates all ForgeTSS metrics.
func Register() {
	registry = prometheus.NewRegistry()

	queueDepth = promauto.With(registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "forgetss",
			Name:      "queue_depth",
			Help:      "Number of pending transactions waiting in the queue",
		},
		[]string{"status"},
	)

	submissionDuration = promauto.With(registry).NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "forgetss",
			Name:      "submission_duration_seconds",
			Help:      "Duration of transaction submission in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.01, 2, 10),
		},
	)

	submissionTotal = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "forgetss",
			Name:      "submission_total",
			Help:      "Total number of submission attempts, labeled by result",
		},
		[]string{"status"},
	)

	channelAccountsAvailable = promauto.With(registry).NewGauge(
		prometheus.GaugeOpts{
			Namespace: "forgetss",
			Name:      "channel_accounts_available",
			Help:      "Number of idle channel accounts in the pool",
		},
	)

	retryTotal = promauto.With(registry).NewCounter(
		prometheus.CounterOpts{
			Namespace: "forgetss",
			Name:      "retry_total",
			Help:      "Total number of transaction retries",
		},
	)
}

// QueueDepth records the number of pending transactions by status.
func QueueDepth(status string, count int) {
	queueDepth.WithLabelValues(status).Set(float64(count))
}

// RecordSubmissionDuration records the duration of a submission attempt.
func RecordSubmissionDuration(duration float64) {
	submissionDuration.Observe(duration)
}

// RecordSubmission records a submission attempt result.
func RecordSubmission(status string) {
	submissionTotal.WithLabelValues(status).Inc()
}

// SetChannelAccountsAvailable sets the count of available (idle) channel accounts.
func SetChannelAccountsAvailable(count int) {
	channelAccountsAvailable.Set(float64(count))
}

// IncrementRetry increments the total retry counter.
func IncrementRetry() {
	retryTotal.Inc()
}

// Registry returns the Prometheus registry for mounting the HTTP handler.
func Registry() *prometheus.Registry {
	return registry
}

var (
	queueDepth             *prometheus.GaugeVec
	submissionDuration     prometheus.Histogram
	submissionTotal        *prometheus.CounterVec
	channelAccountsAvailable prometheus.Gauge
	retryTotal             prometheus.Counter
)
