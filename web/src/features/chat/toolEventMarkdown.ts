import type { ToolUseEvent } from "./types";

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
  const stdout = typeof rec.stdout === "string" ? rec.stdout.trim() : "";
  const stderr = typeof rec.stderr === "string" ? rec.stderr.trim() : "";

  const parts: string[] = [];
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

/** Markdown for a chat tool_event row (mirrors backend ChatToolUseSSE). */
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
  const failed = event.status === "failed" || event.status === "error" || event.status === "blocked";
  if (failed) {
    const body = formatToolResultSummary(event);
    return `工具调用失败：${agent} 使用 **${label}**${argHint}\n\n${event.error || "未知错误"}${body}`;
  }
  const duration = event.duration_ms ? `，耗时 ${event.duration_ms}ms` : "";
  const body = formatToolResultSummary(event);
  return `工具调用完成：${agent} 已使用 **${label}**${argHint}${duration}${body}`;
}
