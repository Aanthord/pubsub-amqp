package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	MessagePublished = promauto.NewCounter(prometheus.CounterOpts{
		Name: "messages_published_total",
		Help: "The total number of published messages",
	})

	MessageReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "messages_received_total",
		Help: "The total number of received messages",
	})

	S3UploadDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "s3_upload_duration_seconds",
		Help:    "The duration of S3 uploads in seconds",
		Buckets: prometheus.DefBuckets,
	})

	S3DownloadDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "s3_download_duration_seconds",
		Help:    "The duration of S3 downloads in seconds",
		Buckets: prometheus.DefBuckets,
	})

	RedshiftQueryDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "redshift_query_duration_seconds",
		Help:    "The duration of Redshift queries in seconds",
		Buckets: prometheus.DefBuckets,
	})

	Neo4jQueryDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "neo4j_query_duration_seconds",
		Help:    "The duration of Neo4j queries in seconds",
		Buckets: prometheus.DefBuckets,
	})

	SearchDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "search_duration_seconds",
		Help:    "The duration of search operations in seconds",
		Buckets: prometheus.DefBuckets,
	})

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "The duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"handler"},
	)

	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "The total number of HTTP requests",
		},
		[]string{"handler"},
	)

	HTTPRequestErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_errors_total",
			Help: "The total number of HTTP request errors",
		},
		[]string{"handler", "code"},
	)
)

func Init() {
	// This function is called to ensure all metrics are registered
	// It doesn't need to do anything as the metrics are automatically registered by promauto
}
