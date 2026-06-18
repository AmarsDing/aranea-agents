/**
 * Centralized timeout and interval constants for chat, WebSocket, and heartbeat.
 * Grouped by domain. Values are in milliseconds unless noted.
 *
 * Configuration layering:
 *   L1 — Code defaults (this file)
 *   L2 — System Settings (future: backend pushes overrides via API)
 *   L3 — Per-session runtime (future: event-driven overrides)
 */

// ── Chat Send ──────────────────────────────────────────────────────────
export const CHAT_SEND_DISPATCH_TIMEOUT_MS = 30_000       // Message send dispatch timeout
export const CHAT_TURN_ACK_TIMEOUT_MS = 30_000            // Client turn-ack timeout
export const CHAT_FIRST_BYTE_TIMEOUT_MS = 90_000          // First byte timeout after run accepted
export const CHAT_RUN_STALL_TIMEOUT_MS = 180_000          // Run stall timeout (no run events)
export const CHAT_RUN_STALL_CHECK_INTERVAL_MS = 30_000    // Stall check polling interval
export const CHAT_STALL_NOTIFY_DURATION_MS = 8_000        // Stall warning notification display duration
export const CHAT_FIRST_BYTE_NOTIFY_DURATION_MS = 8_000   // First byte timeout notification duration

// ── WebSocket Transport ────────────────────────────────────────────────
export const WS_MAX_RECONNECT_DELAY_MS = 30_000           // Max reconnect delay
export const WS_MAX_RECONNECT_ATTEMPTS = 10               // Max reconnect attempts
export const WS_HEARTBEAT_INTERVAL_MS = 25_000            // Business WS ping interval
export const WS_RECONNECT_BASE_DELAY_MS = 1_000           // Reconnect base delay (exponential backoff)
export const WS_RUN_STALE_TIMEOUT_MS = 30_000             // Run stale threshold (no run_heartbeat)

// ── Server Heartbeat ───────────────────────────────────────────────────
export const HEARTBEAT_PING_INTERVAL_MS = 15_000          // Health-check ping interval
export const HEARTBEAT_PONG_TIMEOUT_MS = 90_000           // Pong timeout (server offline threshold)
export const HEARTBEAT_RECONNECT_BASE_DELAY_MS = 1_000    // Reconnect base delay
export const HEARTBEAT_RECONNECT_MAX_DELAY_MS = 30_000    // Reconnect max delay
export const HEARTBEAT_INITIAL_CONNECT_TIMEOUT_MS = 8_000 // Initial connection timeout

// ── Chat UI Debounce / Delay ───────────────────────────────────────────
export const CHAT_HYDRATE_DEBOUNCE_MS = 200               // Inbound message hydration debounce
export const CHAT_REBIND_DELAY_MS = 600                    // Page-visible session rebind delay
export const CHAT_CONTEXT_COMPRESS_NOTIFY_MS = 4_000       // Context compression notification duration
export const CHAT_COMPRESS_SUCCESS_NOTIFY_MS = 4_000       // Manual compress success notification
export const CHAT_COMPRESS_SKIP_NOTIFY_MS = 3_000          // No-compression-needed notification
export const CHAT_COMPRESS_FAIL_NOTIFY_MS = 5_000          // Compression failure notification
export const CHAT_ORCHESTRATION_NOTIFY_MS = 4_000          // Orchestration notification duration
