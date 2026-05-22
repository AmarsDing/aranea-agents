// Package metrics defines all Prometheus metric variables for the Aranea service.
// By living in its own package it can be imported by any layer (service, agent,
// event, cronrunner, etc.) without creating import cycles with internal/server.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// All metrics use the "aranea_" prefix to avoid collisions in shared Prometheus
// deployments.  Metrics are registered once at package init time via promauto.
var (
	// ChatTurnDuration tracks agent chat turn latency by agent and status.
	ChatTurnDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aranea_chat_turn_duration_seconds",
		Help:    "Duration of a chat turn from request to response.",
		Buckets: prometheus.DefBuckets,
	}, []string{"agent_id", "status"})

	// AgentBuildCacheHits counts LRU cache hits for BuildTRPCLLMAgent.
	AgentBuildCacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aranea_agent_build_cache_hits_total",
		Help: "Number of agent build cache hits (LRU).",
	})

	// AgentBuildCacheMisses counts LRU cache misses for BuildTRPCLLMAgent.
	AgentBuildCacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aranea_agent_build_cache_misses_total",
		Help: "Number of agent build cache misses (LRU).",
	})

	// EventBusPublished counts events published to the in-process event bus.
	EventBusPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_event_bus_published_total",
		Help: "Number of events published to the event bus.",
	}, []string{"event_type"})

	// EventBusDropped counts events dropped due to backpressure.
	EventBusDropped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_event_bus_dropped_total",
		Help: "Number of events dropped due to subscriber backpressure.",
	}, []string{"event_type", "policy"})

	// GraphActiveExecutions is a gauge of currently running graph executions.
	GraphActiveExecutions = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aranea_graph_active_executions",
		Help: "Number of graph executions currently in progress.",
	})

	// ToolInvocationTotal counts tool invocations by tool name and outcome.
	ToolInvocationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_tool_invocation_total",
		Help: "Number of tool invocations, labelled by tool name and status.",
	}, []string{"tool", "status"})

	// ProviderRequestTotal counts LLM provider requests by provider, model, and status.
	ProviderRequestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_provider_request_total",
		Help: "Number of LLM provider requests by provider, model, and status.",
	}, []string{"provider", "model", "status"})

	// ProviderRequestDuration tracks LLM provider request latency.
	ProviderRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aranea_provider_request_duration_seconds",
		Help:    "Latency of LLM provider requests.",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
	}, []string{"provider", "model"})

	// PluginInvokeTotal counts plugin invocations by plugin name, callback point, and status.
	PluginInvokeTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_plugin_invoke_total",
		Help: "Number of plugin callback invocations by plugin, point, and status.",
	}, []string{"plugin", "point", "status"})

	// PluginBlockTotal counts plugin-blocked operations by plugin and reason.
	PluginBlockTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_plugin_block_total",
		Help: "Number of operations blocked by a plugin.",
	}, []string{"plugin", "reason"})

	// AutoMemoryJobTotal counts auto-memory job completions by status.
	AutoMemoryJobTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_auto_memory_job_total",
		Help: "Number of auto-memory extraction jobs by final status.",
	}, []string{"status"})

	// AutoMemoryExtractionDuration tracks auto-memory extraction latency.
	AutoMemoryExtractionDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "aranea_auto_memory_extraction_duration_seconds",
		Help:    "Duration of auto-memory extraction per job.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60},
	})

	// ArtifactUploadBytesTotal counts bytes uploaded as artifacts.
	ArtifactUploadBytesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aranea_artifact_upload_bytes_total",
		Help: "Total bytes uploaded as artifacts.",
	})

	// ArtifactDownloadBytesTotal counts bytes downloaded as artifacts.
	ArtifactDownloadBytesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aranea_artifact_download_bytes_total",
		Help: "Total bytes downloaded as artifacts.",
	})

	// ArtifactStorageBytes is a gauge of total artifact bytes on disk.
	ArtifactStorageBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aranea_artifact_storage_bytes",
		Help: "Approximate total bytes used by artifact storage.",
	})

	// MCPSessionReconnectTotal counts MCP transport session reconnect attempts by server and outcome.
	MCPSessionReconnectTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_mcp_session_reconnect_total",
		Help: "MCP session reconnect attempts by server_key and outcome (success, failed, exhausted).",
	}, []string{"server_key", "outcome"})

	// MCPInvocationTotal counts MCP-classified tool invocations by tool name and outcome.
	MCPInvocationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_mcp_invocation_total",
		Help: "MCP tool invocations classified at runtime, labelled by tool name and status.",
	}, []string{"tool", "status"})

	// AlertNotifyTotal counts monitor alert outbound delivery attempts.
	AlertNotifyTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_alert_notify_total",
		Help: "Monitor alert notification delivery by channel type and status.",
	}, []string{"channel", "status"})

	// ModelRouterFallbackTotal counts model_router fallbacks to the base model.
	ModelRouterFallbackTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_model_router_fallback_total",
		Help: "Model router fallbacks to base model by reason.",
	}, []string{"reason"})

	// ChannelDeliveryTotal counts outbound channel delivery attempts by platform and status.
	ChannelDeliveryTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_channel_delivery_total",
		Help: "Channel outbound delivery attempts by platform and status (delivered, retry, dead_letter, invalid).",
	}, []string{"platform", "status"})

	// ChannelDeliveryDuration tracks outbound send latency per platform.
	ChannelDeliveryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aranea_channel_delivery_duration_seconds",
		Help:    "Latency of channel outbound send attempts.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 15, 30},
	}, []string{"platform"})

	// ChannelRuntimeReconnectTotal counts runtime connector disconnect/reconnect cycles.
	ChannelRuntimeReconnectTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_channel_runtime_reconnect_total",
		Help: "Channel runtime connector reconnect events by platform, receive_mode, and outcome.",
	}, []string{"platform", "receive_mode", "outcome"})

	// ChannelRuntimeConnected tracks active long-lived channel connectors.
	ChannelRuntimeConnected = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aranea_channel_runtime_connected",
		Help: "Number of active channel runtime connectors by platform and receive_mode.",
	}, []string{"platform", "receive_mode"})

	// ChannelStreamUpdateTotal counts in-place stream preview updates during channel turns.
	ChannelStreamUpdateTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_channel_stream_update_total",
		Help: "Channel streaming preview updates by platform, phase (delta|flush), and result (ok|error).",
	}, []string{"platform", "phase", "result"})

	// ChannelTurnDuration tracks inbound turn latency from execute start to completion.
	ChannelTurnDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aranea_channel_turn_duration_seconds",
		Help:    "Duration of a channel inbound turn (execute phase).",
		Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600},
	}, []string{"platform"})

	// ChannelTurnJobTotal counts channel turn job state transitions by channel and status.
	ChannelTurnJobTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_channel_turn_job_total",
		Help: "Channel turn job lifecycle events by channel_id and status.",
	}, []string{"channel_id", "status"})

	// ChannelProgressPatchTotal counts IM progress PATCH updates during long turns.
	ChannelProgressPatchTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_channel_progress_patch_total",
		Help: "Channel progress PATCH updates by platform and result (ok|error).",
	}, []string{"platform", "result"})

	// ChannelToolCardTotal counts Feishu interactive tool card build/send attempts.
	ChannelToolCardTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_channel_tool_card_total",
		Help: "Channel IM tool card operations by platform, phase (build|send), and result.",
	}, []string{"platform", "phase", "result"})
)
