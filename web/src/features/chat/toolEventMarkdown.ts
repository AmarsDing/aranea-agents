import type { Message, ToolUseEvent } from "./types";

function truncateBlock(s: string, max: number): string {
  if (s.length <= max) return s;
  return `${s.slice(0, max)}\n\n… (已截断)`;
}

/** Appends stdout/stderr (shell) or a short key summary for other tools. */
function formatToolResultSummary(event: ToolUseEvent): string {
  const r = event.result;
  if (!r || typeof r !== "object" || Array.isArray(r)) {
    return "";
  }
  const rec = r as Record<string, unknown>;
  const parts: string[] = [];

  if (event.tool_name === "list_file") {
    const path = typeof rec.path === "string" ? rec.path : ".";
    const items = rec.items;
    if (Array.isArray(items)) {
      const json = JSON.stringify(items);
      parts.push(
        `\n\n**结果**（${items.length} 项，目录 \`${path}\`）\n\n\`\`\`json\n${truncateBlock(json, 16000)}\n\`\`\``
      );
    }
    return parts.join("");
  }

  if (event.tool_name === "search_content") {
    const matches = rec.matches;
    const trunc = rec.truncated === true ? "（已截断）" : "";
    if (Array.isArray(matches)) {
      const json = JSON.stringify(matches);
      parts.push(`\n\n**结果** search_content ${trunc}（${matches.length} 条）\n\n\`\`\`json\n${truncateBlock(json, 16000)}\n\`\`\``);
    }
    return parts.join("");
  }

  if (event.tool_name === "read_file") {
    const path = typeof rec.path === "string" ? rec.path : "";
    const body = typeof rec.content === "string" ? rec.content : typeof rec.body === "string" ? rec.body : "";
    if (body) {
      parts.push(`\n\n**结果**（\`${path || "file"}\`）\n\n\`\`\`text\n${truncateBlock(body, 24000)}\n\`\`\``);
      return parts.join("");
    }
  }

  const stdout = typeof rec.stdout === "string" ? rec.stdout.trim() : "";
  const stderr = typeof rec.stderr === "string" ? rec.stderr.trim() : "";

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
  if (parts.length === 0 && event.tool_name !== "shell_exec" && event.tool_name !== "shell") {
    const keys = Object.keys(rec).filter((k) => k !== "error").slice(0, 6);
    if (keys.length > 0) {
      const lines = keys.map((k) => {
        const v = rec[k];
        const s = typeof v === "string" ? v : JSON.stringify(v);
        return `- **${k}:** ${truncateBlock(s, 400)}`;
      });
      parts.push(`\n\n**结果**\n${lines.join("\n")}`);
    }
  }
  return parts.join("");
}

/** Markdown for a chat tool_event row (mirrors backend tool_event envelope projection). */
export function formatToolEventMarkdown(event: ToolUseEvent): string {
  const label = event.tool_label || event.tool_name;
  const agent = event.agent_name || event.agent_key || "Agent";
  const path = typeof event.arguments?.path === "string" ? ` \`${event.arguments.path}\`` : "";
  const cmd =
    typeof event.arguments?.command === "string" && event.arguments.command.trim() !== ""
      ? ` \`${String(event.arguments.command).slice(0, 120)}${String(event.arguments.command).length > 120 ? "…" : ""}\``
      : "";
  const argHint = path || cmd;

  if (event.status === "running") {
    return `工具调用：${agent} 正在使用 **${label}**${argHint}`;
  }
  if (event.status === "blocked") {
    return `工具调用待确认：${agent} 使用 **${label}**${argHint}`;
  }
  if (event.status === "cancelled") {
    return `工具调用已取消：${agent} 使用 **${label}**${argHint}`;
  }
  const failed = event.status === "failed" || event.status === "error";
  if (failed) {
    const body = formatToolResultSummary(event);
    return `工具调用失败：${agent} 使用 **${label}**${argHint}\n\n${event.error || "未知错误"}${body}`;
  }
  const duration = event.duration_ms ? `，耗时 ${event.duration_ms}ms` : "";
  const body = formatToolResultSummary(event);
  return `工具调用完成：${agent} 已使用 **${label}**${argHint}${duration}${body}`;
}

export function toolEventToMessage(sessionID: string, event: ToolUseEvent): Message {
  const status = toolEventMessageStatus(event.status);
  const messageId = event.id?.trim() ? `act-${event.id.trim()}` : `tool-${event.agent_id || event.agent_key || "agent"}-${event.id || event.tool_name}`;
  return {
    id: messageId,
    session_id: sessionID,
    parent_message_id: "",
    turn_index: 0,
    role: "assistant",
    content_markdown: formatToolEventMarkdown(event),
    model_name: "",
    token_in: 0,
    token_out: 0,
    latency_ms: event.duration_ms ?? 0,
    status,
    attachments_count: 0,
    options_json: JSON.stringify({
      schema: "chat.activity/v1",
      agent: {
        agent_id: event.agent_id,
        agent_key: event.agent_key,
        name: event.agent_name || event.agent_key,
        icon: event.agent_icon || ""
      },
      tool_event: { ...event, is_long_running: event.is_long_running }
    }),
    error_message: event.error || "",
    created_at: event.occurred_at || new Date().toISOString()
  };
}

function toolEventMessageStatus(status: string): string {
  switch (status) {
    case "running":
      return "tool_running";
    case "blocked":
      return "tool_blocked";
    case "cancelled":
      return "tool_cancelled";
    case "failed":
    case "error":
      return "tool_failed";
    default:
      return "tool_success";
  }
}
