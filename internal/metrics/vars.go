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
	// ChatTTFT tracks time-to-first-token: duration from stream consume start to first meaningful event.
	ChatTTFT = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aranea_chat_ttft_seconds",
		Help:    "Time to first token: duration from stream consume start to first meaningful event.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
	}, []string{"agent_id", "first_byte_type"})

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

	// AutoMemoryLLMFallbackTotal counts consolidator fallbacks from LLM to heuristic extraction.
	AutoMemoryLLMFallbackTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aranea_auto_memory_llm_fallback_total",
		Help: "Number of auto-memory jobs where LLM extraction failed and heuristic fallback was used.",
	})

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

	// ToolArgsGuardTotal counts tool-argument quality outcomes detected by the
	// repair guard (29-token 工具质量度量). outcome ∈ {repaired, invalid}：
	// repaired = 模型产出的坏 JSON 被 guard 修复；invalid = 坏 JSON 不可修复。
	// 与 aranea_tool_invocation_total 相除即得各工具的参数一次合法率。
	ToolArgsGuardTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_tool_args_guard_total",
		Help: "Tool argument quality outcomes by tool (repaired = salvaged JSON, invalid = unrepairable).",
	}, []string{"tool", "outcome"})

	// ContextBudgetTokens observes the per-turn context budget ledger
	// (29-token.design.md §9.6) by category, so static_prefix/tools_schema/
	// history 等分桶的 token 占比可聚合分析（纯日志无闭环）。
	ContextBudgetTokens = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aranea_context_budget_tokens",
		Help:    "Estimated prompt tokens per context budget category per turn.",
		Buckets: []float64{100, 500, 1000, 2500, 5000, 10000, 25000, 50000, 100000},
	}, []string{"category"})

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

	// ChannelBusyIntentTotal counts ingress busy-line intent classifications (CH-BOR-04).
	ChannelBusyIntentTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_channel_busy_intent_total",
		Help: "Channel ingress policy intent by label (cancel|status|queue|reject_busy|route_async|admit).",
	}, []string{"intent"})

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

	// TeamGraphRuntimeTotal counts Team Run runtime path selection (M53 Phase 5).
	TeamGraphRuntimeTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_team_graph_runtime_total",
		Help: "Team run runtime path: graph success, native fallback, or native primary.",
	}, []string{"outcome", "reason"})

	// SkillImportTotal counts skill ZIP import operations by phase and status.
	SkillImportTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_skill_import_total",
		Help: "Number of skill ZIP import operations, labelled by phase and status.",
	}, []string{"phase", "status"})

	// SkillImportDuration tracks skill ZIP import latency by phase.
	SkillImportDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aranea_skill_import_duration_seconds",
		Help:    "Duration of skill ZIP import operations by phase.",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
	}, []string{"phase"})

	SafegoPanicRecovered = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_safego_panic_recovered_total",
		Help: "Number of panics recovered by safego, labelled by goroutine name.",
	}, []string{"name"})

	// Spirit orchestration phase metrics (P3-3).
	// These histograms measure the three Spirit phases (Plan→Allocate→Orchestrate)
	// so operators can locate latency bottlenecks across the orchestration pipeline.
	// See docs/development/70-orchestration-longtask-memory.design.md §7.3.

	// spiritPhaseBuckets covers sub-second to multi-minute planning/allocation phases.
	// DefBuckets (max 10s) are too narrow for Spirit phases that may invoke LLM calls.
	spiritPhaseBuckets = []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300}

	// SpiritPlanDuration tracks the planning phase (intent pass + complexity
	// assessment + task decomposition). Buckets cover sub-second to 5 minutes.
	SpiritPlanDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "aranea_spirit_plan_duration_seconds",
		Help:    "Duration of Spirit planning phase (intent→assess→decompose).",
		Buckets: spiritPhaseBuckets,
	})

	// SpiritAllocDuration tracks the agent allocation phase (4-layer matching +
	// AgentFactory fallback). Buckets cover sub-second to 5 minutes.
	SpiritAllocDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "aranea_spirit_alloc_duration_seconds",
		Help:    "Duration of Spirit agent allocation phase (matching + factory).",
		Buckets: spiritPhaseBuckets,
	})

	// SpiritOrchDuration tracks the orchestration phase (graph/team execution).
	// Buckets extend to 1h to capture 24h long-task sub-phases; the upper bound
	// is intentionally below 24h because a single orchestration phase should
	// not run uninterrupted for the full task lifetime (checkpoints split it).
	SpiritOrchDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "aranea_spirit_orch_duration_seconds",
		Help:    "Duration of Spirit orchestration phase (graph/team execution).",
		Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600},
	})

	// AgentFactoryCreated counts dynamically created agents (P1-4 AgentFactory).
	// Monotonically increasing; used to verify the 4-layer matching fallback rate.
	AgentFactoryCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aranea_agent_factory_created_total",
		Help: "Total number of agents dynamically created by AgentFactory.",
	})

	// GraphReplanTotal counts runtime graph replans by type (P2-2 RuntimeReplanner).
	// Labels: retry / reroute / insert_fallback / rebuild_subgraph.
	// Used to monitor graph stability and replan loop prevention (max 3 replans).
	GraphReplanTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_graph_replan_total",
		Help: "Total number of runtime graph replans by type.",
	}, []string{"type"})

	// SequencerDeadLetterTotal counts v2 sequencer events permanently sent to
	// the dead-letter ring (persist retries exhausted or persist queue full).
	// Any increment means durable event loss — operators should investigate.
	SequencerDeadLetterTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_sequencer_dead_letter_total",
		Help: "Number of v2 events sent to the dead-letter ring after persist failure.",
	}, []string{"event_kind"})

	// SequencerDeadLetterSize is the current occupancy of the v2 sequencer
	// dead-letter ring (capacity 512, FIFO evict).
	SequencerDeadLetterSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aranea_sequencer_dead_letter_size",
		Help: "Current number of events held in the v2 sequencer dead-letter ring.",
	})

	// SequencerDeadLetterReplayTotal counts durable dead-letter replay
	// outcomes (P1-R2b): replayed / failed / abandoned.
	SequencerDeadLetterReplayTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_sequencer_dead_letter_replay_total",
		Help: "Dead-letter replay outcomes from the durable event_dead_letter store.",
	}, []string{"outcome"})
)

// SafegoPanicHook returns a PanicHook function that increments the
// SafegoPanicRecovered Prometheus counter. Register it via
// safego.RegisterPanicHook(metrics.SafegoPanicHook()) during startup.
func SafegoPanicHook() func(name string, r interface{}, stack []byte) {
	return func(name string, _ interface{}, _ []byte) {
		SafegoPanicRecovered.WithLabelValues(name).Inc()
	}
}
