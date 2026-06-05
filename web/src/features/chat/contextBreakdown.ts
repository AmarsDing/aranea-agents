import { formatTokenCount } from './composerUsageMetrics';

/** Local mirror of the former backend PromptTokenBreakdown (removed from envelope contract). */
export type PromptTokenBreakdown = {
  system_prompt?: number;
  skills?: number;
  memory?: number;
  intent_pass?: number;
  session_summary?: number;
  tool_results?: number;
  history?: number;
  user_message?: number;
};

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

// NOTE: color 值为 CSS 变量字符串（var(--chart-color-*)），仅适用于 DOM 内联样式；
// 若需 Canvas/Chart.js 等场景，须通过 getComputedStyle 读取计算值。
export const BREAKDOWN_COLORS: Record<string, string> = {
  system_prompt: 'var(--chart-color-system-prompt)',
  skills: 'var(--chart-color-skills)',
  memory: 'var(--chart-color-memory)',
  intent_pass: 'var(--chart-color-intent-pass)',
  session_summary: 'var(--chart-color-session-summary)',
  tool_results: 'var(--chart-color-tool-results)',
  history: 'var(--chart-color-history)',
  user_message: 'var(--chart-color-user-message)',
};

export const BREAKDOWN_LABELS: Record<string, string> = {
  system_prompt: 'System Prompt',
  skills: 'Skills',
  memory: 'Memory (L1-L4)',
  intent_pass: 'Intent Pass',
  session_summary: 'Session 摘要',
  tool_results: 'Tool Results',
  history: '对话历史',
  user_message: '用户消息',
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
  return sections.filter((s) => s.key.startsWith(keyPrefix)).reduce((sum, s) => sum + s.estTokens, 0);
}

export function computeBreakdown(
  preview: PromptPreviewReport | null,
  contextUsedTokens: number,
  contextWindow: number,
  toolCallCount: number,
  messageCount: number,
  serverBreakdown?: PromptTokenBreakdown,
): PromptBreakdown {
  if (serverBreakdown && Object.values(serverBreakdown).some((v) => v != null && v > 0)) {
    return computeBreakdownFromServer(serverBreakdown, contextUsedTokens, contextWindow);
  }
  return computeBreakdownFromEstimation(preview, contextUsedTokens, contextWindow, toolCallCount, messageCount);
}

function computeBreakdownFromServer(
  bd: PromptTokenBreakdown,
  contextUsedTokens: number,
  contextWindow: number,
): PromptBreakdown {
  const categories: PromptBreakdownCategory[] = [];
  const keys: (keyof PromptTokenBreakdown)[] = [
    'system_prompt',
    'skills',
    'memory',
    'intent_pass',
    'session_summary',
    'tool_results',
    'history',
    'user_message',
  ];
  for (const key of keys) {
    const tokens = bd[key];
    if (tokens != null && tokens > 0) {
      categories.push({
        key,
        label: BREAKDOWN_LABELS[key] ?? key,
        estTokens: tokens,
        color: BREAKDOWN_COLORS[key] ?? 'var(--chart-color-history)',
      });
    }
  }
  const totalPromptTokens = contextUsedTokens > 0 ? contextUsedTokens : categories.reduce((s, c) => s + c.estTokens, 0);
  const ratio = contextWindow > 0 ? Math.min(1, totalPromptTokens / contextWindow) : 0;
  return { categories, totalPromptTokens, contextWindow, contextRatio: ratio };
}

function computeBreakdownFromEstimation(
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
      categories.push({
        key: 'system_prompt',
        label: BREAKDOWN_LABELS.system_prompt,
        estTokens: systemTokens,
        color: BREAKDOWN_COLORS.system_prompt,
      });
      accounted += systemTokens;
    }

    const skillsTokens = sumSections(preview.sections, 'skills');
    if (skillsTokens > 0) {
      categories.push({
        key: 'skills',
        label: BREAKDOWN_LABELS.skills,
        estTokens: skillsTokens,
        color: BREAKDOWN_COLORS.skills,
      });
      accounted += skillsTokens;
    }

    const memoryTokens =
      sumSections(preview.sections, 'l1_memory') +
      sumSections(preview.sections, 'l2_memory') +
      sumSections(preview.sections, 'l3_memory') +
      sumSections(preview.sections, 'l4_memory') +
      sumSections(preview.sections, 'user_memories');
    if (memoryTokens > 0) {
      categories.push({
        key: 'memory',
        label: BREAKDOWN_LABELS.memory,
        estTokens: memoryTokens,
        color: BREAKDOWN_COLORS.memory,
      });
      accounted += memoryTokens;
    }

    const intentTokens = sumSections(preview.sections, 'intent');
    if (intentTokens > 0) {
      categories.push({
        key: 'intent_pass',
        label: BREAKDOWN_LABELS.intent_pass,
        estTokens: intentTokens,
        color: BREAKDOWN_COLORS.intent_pass,
      });
      accounted += intentTokens;
    }

    const summaryTokens = sumSections(preview.sections, 'session_summary');
    if (summaryTokens > 0) {
      categories.push({
        key: 'session_summary',
        label: BREAKDOWN_LABELS.session_summary,
        estTokens: summaryTokens,
        color: BREAKDOWN_COLORS.session_summary,
      });
      accounted += summaryTokens;
    }
  }

  const EST_TOOL_RESULT_TOKENS = 800;
  const EST_USER_MSG_TOKENS = 60;

  const toolResultTokens = toolCallCount * EST_TOOL_RESULT_TOKENS;
  if (toolResultTokens > 0) {
    categories.push({
      key: 'tool_results',
      label: BREAKDOWN_LABELS.tool_results,
      estTokens: toolResultTokens,
      color: BREAKDOWN_COLORS.tool_results,
    });
    accounted += toolResultTokens;
  }

  const residual = Math.max(0, contextUsedTokens - accounted);
  const userMsgTokens = Math.min(residual, Math.max(0, messageCount) * EST_USER_MSG_TOKENS);
  const historyTokens = Math.max(0, residual - userMsgTokens);

  if (userMsgTokens > 0) {
    categories.push({
      key: 'user_message',
      label: BREAKDOWN_LABELS.user_message,
      estTokens: userMsgTokens,
      color: BREAKDOWN_COLORS.user_message,
    });
  }
  if (historyTokens > 0) {
    categories.push({
      key: 'history',
      label: BREAKDOWN_LABELS.history,
      estTokens: historyTokens,
      color: BREAKDOWN_COLORS.history,
    });
  }

  const totalPromptTokens = contextUsedTokens > 0 ? contextUsedTokens : categories.reduce((s, c) => s + c.estTokens, 0);
  const ratio = contextWindow > 0 ? Math.min(1, totalPromptTokens / contextWindow) : 0;

  return { categories, totalPromptTokens, contextWindow, contextRatio: ratio };
}

export function breakdownPercent(tokenCount: number, total: number): string {
  if (total <= 0) return '0%';
  return `${Math.round((tokenCount / total) * 100)}%`;
}

export function formatBreakdownRow(cat: PromptBreakdownCategory, total: number): string {
  return `${cat.label}  ${formatTokenCount(cat.estTokens)}  ${breakdownPercent(cat.estTokens, total)}`;
}
