/**
 * Tool-call status mapping — single source of truth shared between
 * envelope ingestion, agent block composition, and message persistence.
 *
 * Wire → ToolUseEvent → ToolSection / Message status.
 *
 * Wire status values (from the LLM / orchestrator) can be one of:
 *   calling | running | in_progress | failed | error | blocked |
 *   cancelled | interrupted | success
 *
 * The internal canonical form is:
 *   'running' | 'success' | 'failed' | 'blocked' | 'cancelled'
 *
 * The DB / message-row form is MESSAGE_STATUS.* (e.g. 'tool_running').
 */

import { MESSAGE_STATUS } from '../../../domain/types';

/** Internal canonical status of a tool call. */
export type CanonicalToolStatus =
  | 'running'
  | 'success'
  | 'failed'
  | 'blocked'
  | 'cancelled';

const WIRE_TO_CANONICAL: Record<string, CanonicalToolStatus> = {
  calling: 'running',
  running: 'running',
  in_progress: 'running',
  failed: 'failed',
  error: 'failed',
  blocked: 'blocked',
  cancelled: 'cancelled',
  interrupted: 'cancelled',
  success: 'success',
};

/** Normalize a wire-level status string to the canonical internal form. */
export function canonicalToolStatus(wire: string): CanonicalToolStatus {
  return WIRE_TO_CANONICAL[wire.toLowerCase().trim()] ?? 'running';
}

const CANONICAL_TO_MESSAGE: Record<CanonicalToolStatus, string> = {
  running: MESSAGE_STATUS.TOOL_RUNNING,
  success: MESSAGE_STATUS.TOOL_SUCCESS,
  failed: MESSAGE_STATUS.TOOL_FAILED,
  blocked: MESSAGE_STATUS.TOOL_BLOCKED,
  cancelled: MESSAGE_STATUS.TOOL_CANCELLED,
};

/** Map a canonical internal status to the persisted message row status. */
export function messageStatusFromCanonical(canonical: CanonicalToolStatus): string {
  return CANONICAL_TO_MESSAGE[canonical];
}

/** Convenience: wire status string → persisted message status in one hop. */
export function messageStatusFromWire(wire: string): string {
  return messageStatusFromCanonical(canonicalToolStatus(wire));
}
