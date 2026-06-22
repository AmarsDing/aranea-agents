import type { Message, ToolUseEvent } from './types';
import { canonicalToolStatus, messageStatusFromCanonical } from './lib/statusMap';
import { activityMessageId } from './lib/activityMessageId';

function truncateBlock(s: string, max: number): string {
  if (s.length <= max) return s;
  return `${s.slice(0, max)}\n\n… (已截断)`;
}

/** Appends stdout/stderr (shell) or a short key summary for other tools. */
function formatToolResultSummary(event: ToolUseEvent): string {
  const r = event.result;
  if (!r || typeof r !== 'object' || Array.isArray(r)) {
    return '';
  }
  const rec = r as Record<string, unknown>;
  const parts: string[] = [];

  if (event.tool_name === 'list_file') {
    const path = typeof rec.path === 'string' ? rec.path : '.';
    const items = rec.items;
    if (Array.isArray(items)) {
      const json = JSON.stringify(items);
      parts.push(
        `\n\n**结果**（${items.length} 项，目录 \`${path}\`）\n\n\`\`\`json\n${truncateBlock(json, 16000)}\n\`\`\``,
      );
    }
    return parts.join('');
  }

  if (event.tool_name === 'search_content') {
    const matches = rec.matches;
    const trunc = rec.truncated === true ? '（已截断）' : '';
    if (Array.isArray(matches)) {
      const json = JSON.stringify(matches);
      parts.push(
        `\n\n**结果** search_content ${trunc}（${matches.length} 条）\n\n\`\`\`json\n${truncateBlock(json, 16000)}\n\`\`\``,
      );
    }
    return parts.join('');
  }

  if (event.tool_name === 'read_file') {
    const path = typeof rec.path === 'string' ? rec.path : '';
    const body = typeof rec.content === 'string' ? rec.content : typeof rec.body === 'string' ? rec.body : '';
    if (body) {
      parts.push(`\n\n**结果**（\`${path || 'file'}\`）\n\n\`\`\`text\n${truncateBlock(body, 24000)}\n\`\`\``);
      return parts.join('');
    }
  }

  const stdout = typeof rec.stdout === 'string' ? rec.stdout.trim() : '';
  const stderr = typeof rec.stderr === 'string' ? rec.stderr.trim() : '';

  if (stdout) {
    parts.push(`\n\n**输出**\n\n\`\`\`text\n${truncateBlock(stdout, 12000)}\n\`\`\``);
  }
  if (stderr) {
    parts.push(`\n\n**stderr**\n\n\`\`\`text\n${truncateBlock(stderr, 6000)}\n\`\`\``);
  }
  const exitCode = rec.exit_code;
  if (parts.length === 0 && exitCode !== undefined && exitCode !== null) {
    parts.push(`\n\n_exit code:_ \`${String(exitCode)}\`（无终端输出）`);
  }
  if (parts.length === 0 && event.tool_name !== 'shell_exec' && event.tool_name !== 'shell') {
    const keys = Object.keys(rec)
      .filter((k) => k !== 'error')
      .slice(0, 6);
    if (keys.length > 0) {
      const lines = keys.map((k) => {
        const v = rec[k];
        const s = typeof v === 'string' ? v : JSON.stringify(v);
        return `- **${k}:** ${truncateBlock(s, 400)}`;
      });
      parts.push(`\n\n**结果**\n${lines.join('\n')}`);
    }
  }
  return parts.join('');
}

interface ToolArgumentPayload {
  path?: string;
  command?: string;
}

/** Markdown for a chat tool_event row (mirrors backend tool_event envelope projection). */
export function formatToolEventMarkdown(event: ToolUseEvent): string {
  const label = event.tool_label || event.tool_name;
  const agent = event.agent_name || event.agent_key || 'Agent';
  const args = event.arguments as ToolArgumentPayload | undefined;
  const path = typeof args?.path === 'string' ? ` \`${args.path}\`` : '';
  const cmd =
    typeof args?.command === 'string' && args.command.trim() !== ''
      ? ` \`${args.command.slice(0, 120)}${args.command.length > 120 ? '…' : ''}\``
      : '';
  const argHint = path || cmd;

  if (event.status === 'running') {
    return `工具调用：${agent} 正在使用 **${label}**${argHint}`;
  }
  if (event.status === 'blocked') {
    return `工具调用待确认：${agent} 使用 **${label}**${argHint}`;
  }
  if (event.status === 'cancelled') {
    return `工具调用已取消：${agent} 使用 **${label}**${argHint}`;
  }
  const failed = event.status === 'failed' || event.status === 'error';
  if (failed) {
    const body = formatToolResultSummary(event);
    return `工具调用失败：${agent} 使用 **${label}**${argHint}\n\n${event.error || '未知错误'}${body}`;
  }
  const duration = event.duration_ms ? `，耗时 ${event.duration_ms}ms` : '';
  const body = formatToolResultSummary(event);
  return `工具调用完成：${agent} 已使用 **${label}**${argHint}${duration}${body}`;
}

export function toolEventToMessage(sessionID: string, event: ToolUseEvent): Message {
  const status = toolEventMessageStatus(event.status);
  const messageId = activityMessageId(event);
  const agentRef = {
    id: event.agent_id || '',
    agent_key: event.agent_key || '',
    name: event.agent_name || event.agent_key || '',
    icon: '',
  };
  return {
    id: messageId,
    session_id: sessionID,
    parent_message_id: '',
    turn_id: '',
    turn_number: 0,
    seq_in_turn: 0,
    role: 'assistant',
    content_markdown: formatToolEventMarkdown(event),
    model_name: '',
    token_in: 0,
    token_out: 0,
    latency_ms: event.duration_ms ?? 0,
    status,
    attachments_count: 0,
    options_json: JSON.stringify({
      schema: 'chat.activity/v1',
      agent: {
        agent_id: event.agent_id,
        agent_key: event.agent_key,
        name: event.agent_name || event.agent_key,
        icon: '',
      },
      tool_event: { ...event, is_long_running: event.is_long_running },
    }),
    error_message: event.error || '',
    created_at: event.occurred_at || new Date().toISOString(),
    origin: { kind: 'tool_activity', toolEventId: messageId },
    agent_ref: agentRef,
    tool_event: { ...event, is_long_running: event.is_long_running },
  };
}

function toolEventMessageStatus(status: string): string {
  // Delegate to the shared statusMap so the wire→canonical→message mapping
  // is owned by a single module. `status` here is the wire form (the upstream
  // ToolUseEvent.status is still wire-form; canonicalToolStatus accepts either).
  return messageStatusFromCanonical(canonicalToolStatus(status));
}
