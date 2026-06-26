import type { Message } from './types';

export function createPlaceholderMessage(id: string, sessionID: string, role: string, content: string): Message {
  return {
    id,
    session_id: sessionID,
    parent_message_id: '',
    turn_id: '',
    turn_number: 0,
    seq_in_turn: 0,
    role,
    content_markdown: content,
    model_name: 'mock',
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status: 'ok',
    attachments_count: 0,
    options_json: '',
    error_message: '',
    created_at: new Date().toISOString(),
    agent_ref: null,
    team_member: null,
    source_meta: null,
  };
}
