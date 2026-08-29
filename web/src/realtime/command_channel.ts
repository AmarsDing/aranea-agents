/**
 * HTTP Command Channel — B2 channel separation.
 *
 * Responsibility: submit user messages via HTTP and return an ACK only.
 * Does NOT return full message data — all message/state/streaming data
 * arrives via the WS data channel (see `data_channel.ts`).
 *
 * The backend HTTP endpoint still returns full messages; this layer
 * intentionally ignores the response body and extracts only ACK fields
 * (messageId, turnId, status). This enforces the command/data separation:
 *   HTTP = command (fire-and-forget + ACK)
 *   WS   = data (streaming + state + message persistence)
 *
 * Design rationale: mixing HTTP response data with WS streaming data caused
 * dual-source reconciliation bugs (message duplication, ordering conflicts).
 * By making HTTP a pure command channel, the WS data channel becomes the
 * single source of truth for all message/state data.
 */
import { sendMessage } from '../features/chat/api';
import type { SendMessageOptions } from '../features/chat/types';

/** Input for submitting a chat message via the command channel. */
export interface ChatMessageInput {
  session_id: string;
  agent_key?: string;
  team_id?: string;
  content: string;
  /** 提交幂等键（P3）：重试复用同一键，服务端按 session+request_id 去重。 */
  request_id?: string;
  options?: SendMessageOptions;
}

/** ACK returned by the command channel — no full message data. */
export interface MessageAck {
  /** Server-assigned ID of the user message (pre-assigned on accept, SP-1e). */
  messageId: string;
  /** Turn ID the message belongs to (equals messageId for root turns). */
  turnId: string;
  /** 'accepted' = message accepted and processing started; 'queued' = enqueued for later. */
  status: 'queued' | 'accepted';
}

/**
 * Send a chat message via the HTTP command channel.
 *
 * Returns a `MessageAck` with messageId/turnId/status. Full message data
 * (user + assistant content, tool calls, etc.) arrives via the WS data
 * channel — do NOT read message content from the HTTP response.
 *
 * On failure, throws `ChatApiError` (from `features/chat/api.ts`).
 */
export async function sendCommand(message: ChatMessageInput): Promise<MessageAck> {
  return sendMessage(message);
}
