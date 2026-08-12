/**
 * MonitorEvent type system — the transport contract for monitor-channel
 * events (log, flow_log, mcp, alert) pushed by the backend via WS.
 *
 * Backend source of truth:
 *   The backend sends monitor events as `monitor_event?` on the WS protocol's
 *   "monitor" channel. This replaces the legacy `envelope?`-based dispatch
 *   for monitor/log/flow_log events.
 *
 * Field naming follows snake_case to match the backend JSON serialization
 * convention (consistent with ActivityEvent and Envelope).
 */

/**
 * MonitorEventType classifies the semantic type of a monitor event.
 * Aligned with the backend's monitor event type strings.
 */
export type MonitorEventType =
  | 'log'
  | 'flow_log'
  | 'mcp.session.reconnect'
  | 'mcp.health.alert'
  | 'alert.notify'
  | 'monitor.auto_healed'
  | 'monitor.self_check_completed'
  | 'cron.dead_letter'
  | 'skill.reload'
  | 'skill.filesystem.updated'
  | 'skill.filesystem.recovered'
  | 'skill.filesystem.imported'
  | 'computeruse.step';

/**
 * MonitorEvent is the unified transport format for monitor-channel events.
 *
 * Carries log/flow_log/mcp/alert events that are NOT part of the chat
 * Activity timeline. These events drive the monitor panel, health alerts,
 * and operational visibility features.
 */
export interface MonitorEvent {
  /** Unique event identifier. */
  id: string;
  /** Semantic type of the monitor event. */
  type: MonitorEventType;
  /** Emission timestamp (RFC 3339 / ISO 8601). */
  timestamp: string;
  /** Log level (for log/flow_log events). */
  level?: string;
  /** Human-readable log message. */
  message?: string;
  /** Owning session ID (if applicable). */
  session_id?: string;
  /** Emission source (e.g. "skill.watch", "chat-service"). */
  source?: string;
  /** Type-specific metadata. */
  metadata?: Record<string, unknown>;
}

/**
 * Type guard: checks whether an unknown message is a MonitorEvent.
 *
 * Validates that the value is an object with `type` and `id` string fields.
 * Further field validation is deferred to consumers to keep this guard cheap
 * for high-frequency log events.
 */
export function isMonitorEvent(msg: unknown): msg is MonitorEvent {
  return typeof msg === 'object' && msg !== null && 'type' in msg && 'id' in msg;
}

/**
 * Checks whether a MonitorEvent is a log-type event (log or flow_log).
 */
export function isLogEvent(ev: MonitorEvent): boolean {
  return ev.type === 'log' || ev.type === 'flow_log';
}

/**
 * Checks whether a MonitorEvent is an alert-type event
 * (alert.notify or mcp.health.alert).
 */
export function isAlertEvent(ev: MonitorEvent): boolean {
  return ev.type === 'alert.notify' || ev.type === 'mcp.health.alert';
}
