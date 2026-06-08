package biz

// QueryLimits centralizes all hardcoded query limit constants used across the codebase.
// These values control batch sizes, page sizes, and list limits for various queries.
// They are NOT user-configurable but should be maintained in one place for consistency.

// ── Spirit ──────────────────────────────────────────────
const (
	SpiritAgentQueryLimit    = 1  // Number of spirit agents to query (key=__spirit__)
	SpiritRecentMessageCount = 10 // Recent messages to fetch for spirit context
	SpiritRecentRunCount     = 10 // Recent runs to fetch for spirit context
	SpiritCancelSessionLimit = 10 // Sessions to search when canceling a team
)

// ── Session ─────────────────────────────────────────────
const (
	MessageListDefaultLimit = 100  // Default message list page size
	MessageListMaxLimit     = 500  // Maximum message list page size
	TimelineDefaultInvLimit = 100  // Default timeline invocation limit
	TimelineMaxInvLimit     = 500  // Maximum timeline invocation limit
	TimelineMessageMaxFetch = 2000 // Maximum messages to fetch for timeline
	CompressMessageMaxRows  = 512  // Maximum rows for message compression
	ActivityCancelScanLimit = 64   // Maximum activities to scan for cancellation
)

// ── Agent ───────────────────────────────────────────────
const (
	AgentCapabilityLimit = 200  // Agents to load for capability building
	AgentMatchLimit      = 200  // Agents to search for matching
	AgentExportLimit     = 1000 // Agents to export per position
	AgentRoleSearchLimit = 100  // Agents to search by role
	AgentAllToolsLimit   = 1000 // Tools to load for effective tools
)

// ── Monitor ─────────────────────────────────────────────
const (
	SelfHealRecordLimit  = 1000 // Self-heal records to list
	PatternMiningLimit   = 1000 // Records for pattern mining
	DiagBundleEventLimit = 500  // Events for diagnostic bundle
	DiagBundleAlertLimit = 50   // Alerts for diagnostic bundle
)

// ── Experience ──────────────────────────────────────────
const (
	ExperienceQueryLimit = 500 // Experience analytics query limit
)

// ── Channel ─────────────────────────────────────────────
const (
	MaxChannelTurnJobListLimit = 200 // Maximum channel turn jobs to list
)

// ── Plugin ──────────────────────────────────────────────
const (
	PluginReloadLimit = 200 // Plugins to reload
)

// ── Cron ────────────────────────────────────────────────
const (
	CronAgentListLimit = 500 // Agents to list for cron parsing
)

// ── Admin ───────────────────────────────────────────────
const (
	AdminListDefaultLimit = 20 // Default admin list page size
)
