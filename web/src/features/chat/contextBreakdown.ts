import { formatTokenCount } from "./composerUsageMetrics";

export type PromptBreakdownCategory = {
  key: string;
  label: string;
  estTokens: number;
  color: string;
};

export type PromptBreakdown = {
  categories: PromptBreakdownCategory[];
  totalPromptTokens: number;
  contextWindow: number;
  contextRatio: number;
};

export const BREAKDOWN_COLORS: Record<string, string> = {
  system_prompt: "#E9A23B",
  skills: "#8B5CF6",
  memory: "#06B6D4",
  intent_pass: "#F59E0B",
  session_summary: "#10B981",
  tool_results: "#EF4444",
  history: "#6B7280",
  user_message: "#EC4899",
};

export const BREAKDOWN_LABELS: Record<string, string> = {
  system_prompt: "System Prompt",
  skills: "Skills",
  memory: "Memory (L1-L4)",
  intent_pass: "Intent Pass",
  session_summary: "Session 摘要",
  tool_results: "Tool Results",
  history: "对话历史",
  user_message: "用户消息",
};

export type PromptPreviewSection = {
  key: string;
  label: string;
  estTokens: number;
  source: string;
};

export type PromptPreviewInput = {
  sections: Array<{ key: string; label: string; est_tokens: number; source: string }>;
  static_total_tokens: number;
  runtime_overlay_est_tokens: number;
};

export type PromptPreviewReport = {
  sections: PromptPreviewSection[];
  staticTotalTokens: number;
  runtimeOverlayEst: number;
};

export function mapPreviewToReport(preview: PromptPreviewInput): PromptPreviewReport {
  return {
    sections: preview.sections.map((s) => ({
      key: s.key,
      label: s.label,
      estTokens: s.est_tokens,
      source: s.source,
    })),
    staticTotalTokens: preview.static_total_tokens,
    runtimeOverlayEst: preview.runtime_overlay_est_tokens,
  };
}

function sumSections(sections: PromptPreviewSection[], keyPrefix: string): number {
  return sections
    .filter((s) => s.key.startsWith(keyPrefix))
    .reduce((sum, s) => sum + s.estTokens, 0);
}

export function computeBreakdown(
  preview: PromptPreviewReport | null,
  contextUsedTokens: number,
  contextWindow: number,
  toolCallCount: number,
  messageCount: number,
): PromptBreakdown {
  const categories: PromptBreakdownCategory[] = [];

  let accounted = 0;

  if (preview) {
    const systemTokens = preview.staticTotalTokens;
    if (systemTokens > 0) {
      categories.push({ key: "system_prompt", label: BREAKDOWN_LABELS.system_prompt, estTokens: systemTokens, color: BREAKDOWN_COLORS.system_prompt });
      accounted += systemTokens;
    }

    const skillsTokens = sumSections(preview.sections, "skills");
    if (skillsTokens > 0) {
      categories.push({ key: "skills", label: BREAKDOWN_LABELS.skills, estTokens: skillsTokens, color: BREAKDOWN_COLORS.skills });
      accounted += skillsTokens;
    }

    const memoryTokens = sumSections(preview.sections, "l1_memory")
      + sumSections(preview.sections, "l2_memory")
      + sumSections(preview.sections, "l3_memory")
      + sumSections(preview.sections, "l4_memory")
      + sumSections(preview.sections, "user_memories");
    if (memoryTokens > 0) {
      categories.push({ key: "memory", label: BREAKDOWN_LABELS.memory, estTokens: memoryTokens, color: BREAKDOWN_COLORS.memory });
      accounted += memoryTokens;
    }

    const intentTokens = sumSections(preview.sections, "intent");
    if (intentTokens > 0) {
      categories.push({ key: "intent_pass", label: BREAKDOWN_LABELS.intent_pass, estTokens: intentTokens, color: BREAKDOWN_COLORS.intent_pass });
      accounted += intentTokens;
    }

    const summaryTokens = sumSections(preview.sections, "session_summary");
    if (summaryTokens > 0) {
      categories.push({ key: "session_summary", label: BREAKDOWN_LABELS.session_summary, estTokens: summaryTokens, color: BREAKDOWN_COLORS.session_summary });
      accounted += summaryTokens;
    }
  }

  const avgToolResultTokens = 800;
  const toolResultTokens = toolCallCount * avgToolResultTokens;
  if (toolResultTokens > 0) {
    categories.push({ key: "tool_results", label: BREAKDOWN_LABELS.tool_results, estTokens: toolResultTokens, color: BREAKDOWN_COLORS.tool_results });
    accounted += toolResultTokens;
  }

  const residual = Math.max(0, contextUsedTokens - accounted);
  const userMsgTokens = Math.min(residual, Math.max(0, messageCount) * 60);
  const historyTokens = Math.max(0, residual - userMsgTokens);

  if (userMsgTokens > 0) {
    categories.push({ key: "user_message", label: BREAKDOWN_LABELS.user_message, estTokens: userMsgTokens, color: BREAKDOWN_COLORS.user_message });
  }
  if (historyTokens > 0) {
    categories.push({ key: "history", label: BREAKDOWN_LABELS.history, estTokens: historyTokens, color: BREAKDOWN_COLORS.history });
  }

  const totalPromptTokens = contextUsedTokens > 0 ? contextUsedTokens : categories.reduce((s, c) => s + c.estTokens, 0);
  const ratio = contextWindow > 0 ? Math.min(1, totalPromptTokens / contextWindow) : 0;

  return { categories, totalPromptTokens, contextWindow, contextRatio: ratio };
}

export function breakdownPercent(tokenCount: number, total: number): string {
  if (total <= 0) return "0%";
  return `${Math.round((tokenCount / total) * 100)}%`;
}

export function formatBreakdownRow(cat: PromptBreakdownCategory, total: number): string {
  return `${cat.label}  ${formatTokenCount(cat.estTokens)}  ${breakdownPercent(cat.estTokens, total)}`;
}
