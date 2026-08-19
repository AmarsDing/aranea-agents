/**
 * Centralized timeout and interval constants for chat, WebSocket, and heartbeat.
 * Grouped by domain. Values are in milliseconds unless noted.
 *
 * Configuration layering:
 *   L1 — Code defaults (this file)
 *   L2 — System Settings (future: backend pushes overrides via API)
 *   L3 — Per-session runtime (future: event-driven overrides)
 *
 * No-Timeout principle (T1.5): dispatch / turn-ack / first-byte / stream-stall
 * timeouts have been removed. Tasks run until completion or user cancel.
 * The stall check remains as a notification-only mechanism — it never marks
 * messages as failed or interrupts the run.
 */

// ── Chat Send (notification-only, no timeouts) ─────────────────────────
// T1.5: Removed CHAT_SEND_DISPATCH_TIMEOUT_MS, CHAT_TURN_ACK_TIMEOUT_MS,
// CHAT_FIRST_BYTE_TIMEOUT_MS, CHAT_RUN_STALL_TIMEOUT_MS.
// The stall check is now a periodic notification, not a timeout that
// interrupts the run.
export const CHAT_RUN_STALL_CHECK_INTERVAL_MS = 30_000; // Stall check polling interval
export const CHAT_RUN_STALL_NOTIFY_THRESHOLD_MS = 60_000; // Show stall warning after this delay
export const CHAT_STALL_NOTIFY_DURATION_MS = 8_000; // Stall warning notification display duration
export const CHAT_FIRST_BYTE_NOTIFY_THRESHOLD_MS = 30_000; // Show "model thinking" notice after this delay
export const CHAT_FIRST_BYTE_NOTIFY_DURATION_MS = 8_000; // First byte notice display duration

// ── WebSocket Transport ────────────────────────────────────────────────
export const WS_MAX_RECONNECT_DELAY_MS = 30_000; // Max reconnect delay
// T1.8: WS_MAX_RECONNECT_ATTEMPTS removed — unlimited reconnect for
// same-machine deployment. Exponential backoff caps the delay at 30s.
export const WS_HEARTBEAT_INTERVAL_MS = 25_000; // Business WS ping interval
export const WS_RECONNECT_BASE_DELAY_MS = 1_000; // Reconnect base delay (exponential backoff)
// P3.2: zombie (half-open) connection threshold — any downstream frame resets
// the clock; 2 ping intervals + grace covers normal server pong latency.
export const WS_ZOMBIE_TIMEOUT_MS = 55_000;

// ── Server Heartbeat ───────────────────────────────────────────────────
export const HEARTBEAT_PING_INTERVAL_MS = 15_000; // Health-check ping interval
export const HEARTBEAT_PONG_TIMEOUT_MS = 90_000; // Pong timeout (server offline threshold)
export const HEARTBEAT_RECONNECT_BASE_DELAY_MS = 1_000; // Reconnect base delay
export const HEARTBEAT_RECONNECT_MAX_DELAY_MS = 30_000; // Reconnect max delay
export const HEARTBEAT_INITIAL_CONNECT_TIMEOUT_MS = 8_000; // Initial connection timeout
// DEV-only: after a server_shutdown, poll /healthz waiting for the dev backend
// to finish restarting before deciding whether the session must be dropped.
export const HEARTBEAT_SHUTDOWN_RECOVERY_POLL_MS = 2_000; // Recovery health poll interval
export const HEARTBEAT_SHUTDOWN_RECOVERY_TIMEOUT_MS = 60_000; // Max wait for backend recovery

// ── Chat UI Debounce / Delay ───────────────────────────────────────────
export const CHAT_HYDRATE_DEBOUNCE_MS = 200; // Inbound message hydration debounce
export const CHAT_REBIND_DELAY_MS = 600; // Page-visible session rebind delay
export const CHAT_CONTEXT_COMPRESS_NOTIFY_MS = 4_000; // Context compression notification duration
export const CHAT_COMPRESS_SUCCESS_NOTIFY_MS = 4_000; // Manual compress success notification
export const CHAT_COMPRESS_SKIP_NOTIFY_MS = 3_000; // No-compression-needed notification
export const CHAT_COMPRESS_FAIL_NOTIFY_MS = 5_000; // Compression failure notification
export const CHAT_ORCHESTRATION_NOTIFY_MS = 4_000; // Orchestration notification duration
