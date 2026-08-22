import type { ContextBudgetSnapshot, ContextBudgetToolSize } from '../session/types';

/**
 * Prompt-assembly budget (backend ContextBudgetPayload) → UI rows.
 *
 * Pure parsing + view-model builders for the SpiritStatusBar "Context Usage"
 * popup (Cursor 风格分项可观测性). Categories mirror the backend ledger keys
 * in internal/agent/context_budget.go — keep the two in sync.
 */

/** Backend budget category keys (internal/agent/context_budget.go). */
export const CONTEXT_BUDGET_CATEGORY = {
  staticPrefix: 'static_prefix',
  toolsSchema: 'tools_schema',
  memoryL1: 'memory_l1',
  memoryL4: 'memory_l4',
  memoryComposite: 'memory_composite',
  knowledgeCue: 'knowledge_cue',
  skillGuidance: 'skill_guidance',
  skillOverview: 'skill_overview',
  history: 'history',
  otherDynamic: 'other_dynamic',
  toolCatalogCue: 'tool_catalog_cue',
} as const;

export type ContextBudgetRow = {
  /** Backend category key. */
  key: string;
  /** i18n label key under the `chat.` namespace. */
  labelKey: string;
  estTokens: number;
  /** CSS color value (theme chart var). */
  color: string;
};

/**
 * Display order + color mapping for the stacked bar and the row list.
 * Memory L1/L4/复合 deliberate separate rows (产品决策 2026-08-22).
 * Colors reuse the theme `--chart-color-*` tokens (see css/theme/_css-vars-*.sass).
 */
const ROW_DEFS: Array<{ key: string; labelKey: string; color: string }> = [
  {
    key: CONTEXT_BUDGET_CATEGORY.staticPrefix,
    labelKey: 'contextBudgetStaticPrefix',
    color: 'var(--chart-color-system-prompt)',
  },
  {
    key: CONTEXT_BUDGET_CATEGORY.toolsSchema,
    labelKey: 'contextBudgetToolsSchema',
    color: 'var(--chart-color-tools-schema)',
  },
  {
    key: CONTEXT_BUDGET_CATEGORY.skillOverview,
    labelKey: 'contextBudgetSkillOverview',
    color: 'var(--chart-color-skills)',
  },
  {
    key: CONTEXT_BUDGET_CATEGORY.skillGuidance,
    labelKey: 'contextBudgetSkillGuidance',
    color: 'var(--chart-color-intent-pass)',
  },
  { key: CONTEXT_BUDGET_CATEGORY.memoryL1, labelKey: 'contextBudgetMemoryL1', color: 'var(--chart-color-memory)' },
  {
    key: CONTEXT_BUDGET_CATEGORY.memoryL4,
    labelKey: 'contextBudgetMemoryL4',
    color: 'var(--chart-color-session-summary)',
  },
  {
    key: CONTEXT_BUDGET_CATEGORY.memoryComposite,
    labelKey: 'contextBudgetMemoryComposite',
    color: 'var(--chart-color-user-message)',
  },
  {
    key: CONTEXT_BUDGET_CATEGORY.knowledgeCue,
    labelKey: 'contextBudgetKnowledgeCue',
    color: 'var(--chart-color-knowledge-cue)',
  },
  {
    key: CONTEXT_BUDGET_CATEGORY.toolCatalogCue,
    labelKey: 'contextBudgetToolCatalogCue',
    color: 'var(--chart-color-tool-results)',
  },
  { key: CONTEXT_BUDGET_CATEGORY.history, labelKey: 'contextBudgetHistory', color: 'var(--chart-color-history)' },
  {
    key: CONTEXT_BUDGET_CATEGORY.otherDynamic,
    labelKey: 'contextBudgetOtherDynamic',
    color: 'var(--chart-color-other-dynamic)',
  },
];

function numOrNull(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null;
}

/** Parse activity.meta.context_budget into a typed snapshot; null when absent/malformed. */
export function parseContextBudgetMeta(meta: unknown): ContextBudgetSnapshot | null {
  if (!meta || typeof meta !== 'object') return null;
  const raw = meta as Record<string, unknown>;
  const estTokensRaw = raw.est_tokens;
  if (!estTokensRaw || typeof estTokensRaw !== 'object') return null;
  const estTokens: Record<string, number> = {};
  for (const [key, value] of Object.entries(estTokensRaw as Record<string, unknown>)) {
    const n = numOrNull(value);
    if (n != null && n > 0) estTokens[key] = n;
  }
  const total = numOrNull(raw.est_total_input) ?? Object.values(estTokens).reduce((sum, n) => sum + n, 0);
  const toolsCount = numOrNull(raw.tools_count) ?? 0;
  if (total <= 0 && toolsCount <= 0) return null;
  const snapshot: ContextBudgetSnapshot = {
    est_tokens: estTokens,
    est_total_input: total,
    tools_count: toolsCount,
  };
  if (Array.isArray(raw.top_tools)) {
    const tops: ContextBudgetToolSize[] = [];
    for (const item of raw.top_tools) {
      if (!item || typeof item !== 'object') continue;
      const t = item as Record<string, unknown>;
      const name = typeof t.name === 'string' ? t.name : '';
      const tokens = numOrNull(t.est_tokens);
      if (name && tokens != null && tokens > 0) tops.push({ name, est_tokens: tokens });
    }
    if (tops.length) snapshot.top_tools = tops;
  }
  return snapshot;
}

/** Build the ordered, non-zero rows for the popup list + stacked bar. */
export function buildContextBudgetRows(budget: ContextBudgetSnapshot | null | undefined): ContextBudgetRow[] {
  if (!budget) return [];
  const rows: ContextBudgetRow[] = [];
  for (const def of ROW_DEFS) {
    const tokens = budget.est_tokens[def.key] ?? 0;
    if (tokens > 0) {
      rows.push({ key: def.key, labelKey: def.labelKey, estTokens: tokens, color: def.color });
    }
  }
  return rows;
}

/** Sum of all category tokens actually rendered as rows (may differ from est_total_input). */
export function contextBudgetRowsTotal(rows: ContextBudgetRow[]): number {
  return rows.reduce((sum, r) => sum + r.estTokens, 0);
}
