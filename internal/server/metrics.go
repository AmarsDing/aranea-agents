package server

import (
	"net/http"

	// Re-export all metric vars via type aliases so existing callers of this
	// package continue to compile without change.
	// New code should import aranea-agents/internal/metrics directly.
	arametrics "aranea-agents/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ---- Aranea Prometheus metrics (re-exported from internal/metrics) ----
// All metrics use the "aranea_" prefix to avoid collisions in shared Prometheus
// deployments.  Metrics are registered once at package init time via promauto.

var (
	ChatTurnDuration             = arametrics.ChatTurnDuration
	AgentBuildCacheHits          = arametrics.AgentBuildCacheHits
	AgentBuildCacheMisses        = arametrics.AgentBuildCacheMisses
	EventBusPublished            = arametrics.EventBusPublished
	EventBusDropped              = arametrics.EventBusDropped
	GraphActiveExecutions        = arametrics.GraphActiveExecutions
	ToolInvocationTotal          = arametrics.ToolInvocationTotal
	ProviderRequestTotal         = arametrics.ProviderRequestTotal
	ProviderRequestDuration      = arametrics.ProviderRequestDuration
	PluginInvokeTotal            = arametrics.PluginInvokeTotal
	PluginBlockTotal             = arametrics.PluginBlockTotal
	AutoMemoryJobTotal           = arametrics.AutoMemoryJobTotal
	AutoMemoryExtractionDuration = arametrics.AutoMemoryExtractionDuration
	ArtifactUploadBytesTotal     = arametrics.ArtifactUploadBytesTotal
	ArtifactDownloadBytesTotal   = arametrics.ArtifactDownloadBytesTotal
	ArtifactStorageBytes         = arametrics.ArtifactStorageBytes
)

// Ensure the prometheus import is used (for prometheus.HistogramOpts etc. in
// packages that still reference this file's symbol table).
var _ = prometheus.DefBuckets

// RegisterMetricsHandler mounts the Prometheus metrics endpoint at /metrics on
// the given standard net/http mux.  The /metrics path deliberately bypasses the
// auth middleware so that Prometheus scrapers can reach it without credentials.
func RegisterMetricsHandler(mux *http.ServeMux) {
	mux.Handle("/metrics", promhttp.Handler())
}
