/**
 * Centralized query limit constants used across the frontend.
 * These values control API query page sizes, list limits, and local cache caps.
 * They are NOT user-configurable but should be maintained in one place for consistency.
 */

// ── Spirit ──────────────────────────────────────────────
export const SPIRIT_MAX_PARALLEL_TEAMS = 3;
export const SPIRIT_KNOWLEDGE_LIST_LIMIT = 50;

// ── Session ─────────────────────────────────────────────
export const SESSION_LIST_LIMIT = 20;
export const SESSION_TURNS_LIMIT = 20;
/** 聊天侧边栏会话列表每页条数（滚动分页加载） */
export const CHAT_SESSION_PAGE_SIZE = 50;
/** 聊天侧边栏单页上限，与后端 SearchSessions MaxSessionSearchLimit 一致 */
export const CHAT_SESSION_MAX_LIMIT = 500;

// ── Agent ───────────────────────────────────────────────
export const AGENT_LIST_LIMIT = 20; // rows per page
export const AGENT_FULL_LIST_LIMIT = 200; // for dropdowns/associations

// ── Team ────────────────────────────────────────────────
export const TEAM_RUNS_LIMIT = 50;
export const TEAM_RUNS_LOCAL_MAX = 30;
export const TEAM_AGENT_LIST_LIMIT = 1000;
export const TEAM_RUNS_FULL_LIMIT = 200;

// ── Monitor ─────────────────────────────────────────────
export const MONITOR_TRACES_LIMIT = 100;
export const MONITOR_RUNNER_WINDOW_MINUTES = 60;
export const MONITOR_REPORTS_LIMIT = 20;
export const MONITOR_AUDIT_LOG_LIMIT = 200;
/** 审计页服务端分页默认每页条数（与 AuditTable 初始 pageSize 一致） */
export const AUDIT_DEFAULT_PAGE_SIZE = 12;
/** Traces 页服务端分页默认每页条数（与 TraceList 初始 pageSize 一致） */
export const MONITOR_TRACES_PAGE_SIZE = 12;
export const MONITOR_TRACE_EVENT_LIMIT = 100;
export const MONITOR_FLOW_LOG_LIMIT = 500;
export const MONITOR_FLOW_LOG_LOCAL_MAX = 500;
export const MONITOR_REALTIME_EVENT_LOCAL_MAX = 1000;

// ── Memory ──────────────────────────────────────────────
export const MEMORY_SNAPSHOT_LIMIT = 20;
export const MEMORY_ENTITY_LIMIT = 50;
export const MEMORY_EVOLUTION_LIMIT = 20;
export const MEMORY_CASCADE_LIMIT = 30;

// ── Channel ─────────────────────────────────────────────
export const CHANNEL_TURN_JOBS_LIMIT = 30;
export const CHANNEL_DELIVERIES_LIMIT = 30;
export const CHANNEL_ASSOCIATION_LIMIT = 200;

// ── Tools ───────────────────────────────────────────────
export const TOOL_RUNS_PAGE_SIZE = 20;
export const TOOL_ASSOCIATION_LIMIT = 200;

// ── Graph ───────────────────────────────────────────────
export const GRAPH_LIST_PAGE_SIZE = 50;
export const GRAPH_EXECUTION_PAGE_SIZE = 30;
export const GRAPH_CHECKPOINT_LIMIT = 50;
export const GRAPH_LOG_PAGE_SIZE = 100;
export const GRAPH_TASK_LOG_PAGE_SIZE = 100;

// ── Model Catalog ───────────────────────────────────────
export const MODEL_SYNC_LOG_LIMIT = 30;
export const MODEL_PROVIDER_LIMIT = 200;
export const MODEL_BLOCK_SEARCH_LIMIT = 10;
export const MODEL_JSON_SEARCH_CAP = 200;

// ── Evaluation ──────────────────────────────────────────
export const EVALUATION_AGENT_LIMIT = 200;

// ── Knowledge ───────────────────────────────────────────
export const KNOWLEDGE_COLLECTION_LIMIT = 100;
export const KNOWLEDGE_DOCUMENT_LIMIT = 100;

// ── Cron ────────────────────────────────────────────────
export const CRON_TASK_LIMIT = 200;

// ── Plugins ─────────────────────────────────────────────
export const PLUGIN_AGENT_LIMIT = 200;

// ── A2A ─────────────────────────────────────────────────
/** @deprecated Prefer page-size driven ListAudit; kept for callers that still request a single window. */
export const A2A_AUDIT_LIMIT = 100;
export const A2A_AUDIT_PAGE_SIZE_DEFAULT = 10;

// ── Evaluation ──────────────────────────────────────────
export const EVAL_RUNS_PAGE_SIZE_DEFAULT = 8;
export const EVAL_RESULTS_PAGE_SIZE_DEFAULT = 10;

// ── Platform / Skills ───────────────────────────────────
export const SKILL_FULL_LIST_LIMIT = 500;
export const PLATFORM_MODEL_USAGE_LIMIT = 200;

// ── Usage ───────────────────────────────────────────────
/** @deprecated Prefer page-size + offset ListUsageEvents; kept for export/window caps. */
export const USAGE_EVENTS_LIMIT = 200;
export const USAGE_EVENTS_PAGE_SIZE_DEFAULT = 20;

// ── Chat ────────────────────────────────────────────────
export const CHAT_ENVELOPE_LOG_LOCAL_MAX = 500;
export const CHAT_BACKGROUND_JOBS_LIMIT = 50;

// ── Notifications ───────────────────────────────────────
export const INBOUND_NOTIFICATION_MAX_ITEMS = 20;
