package data

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// DB query and connection pool metrics for the data layer.
// Registered once at package init time via promauto.
var (
	// DBQueryDuration tracks database query latency by repo, operation, and status.
	DBQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aranea_db_query_duration_seconds",
		Help:    "Duration of database queries in the data layer.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"repo", "operation", "status"})

	// DBPoolOpenConnections tracks the number of open connections in each pool.
	DBPoolOpenConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aranea_db_pool_open_connections",
		Help: "Number of open database connections in the pool.",
	}, []string{"pool"})

	// DBPoolInUseConnections tracks the number of connections currently in use.
	DBPoolInUseConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aranea_db_pool_in_use_connections",
		Help: "Number of database connections currently in use.",
	}, []string{"pool"})

	// DBPoolIdleConnections tracks the number of idle connections in the pool.
	DBPoolIdleConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aranea_db_pool_idle_connections",
		Help: "Number of idle database connections in the pool.",
	}, []string{"pool"})

	// DBPoolWaitCount tracks the total number of connections waited for.
	DBPoolWaitCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_db_pool_wait_count_total",
		Help: "Total number of connections waited for in the pool.",
	}, []string{"pool"})

	// DBPoolWaitDuration tracks the total time blocked waiting for a connection.
	DBPoolWaitDuration = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_db_pool_wait_duration_seconds_total",
		Help: "Total time blocked waiting for a database connection.",
	}, []string{"pool"})
)

// observeDBQuery records a database query duration with the given labels.
func observeDBQuery(repo, operation string, startSeconds float64, err error) {
	status := "ok"
	if err != nil {
		status = "error"
	}
	DBQueryDuration.WithLabelValues(repo, operation, status).Observe(startSeconds)
}
