import { describe, expect, it } from 'vitest';
import {
  CONTEXT_BUDGET_CATEGORY,
  buildContextBudgetRows,
  contextBudgetRowsTotal,
  parseContextBudgetMeta,
} from '../contextBudget';

describe('parseContextBudgetMeta', () => {
  it('parses a full backend payload', () => {
    const snap = parseContextBudgetMeta({
      est_tokens: { static_prefix: 900, tools_schema: 10900, history: 105900 },
      est_total_input: 117700,
      tools_count: 12,
      top_tools: [
        { name: 'gns3_exec', est_tokens: 3200 },
        { name: 'skill_run', est_tokens: 1500 },
      ],
    });
    expect(snap).not.toBeNull();
    expect(snap?.est_tokens.static_prefix).toBe(900);
    expect(snap?.est_total_input).toBe(117700);
    expect(snap?.tools_count).toBe(12);
    expect(snap?.top_tools).toHaveLength(2);
    expect(snap?.top_tools?.[0]).toEqual({ name: 'gns3_exec', est_tokens: 3200 });
  });

  it('returns null for absent / malformed payloads', () => {
    expect(parseContextBudgetMeta(undefined)).toBeNull();
    expect(parseContextBudgetMeta(null)).toBeNull();
    expect(parseContextBudgetMeta('x')).toBeNull();
    expect(parseContextBudgetMeta({})).toBeNull();
    expect(parseContextBudgetMeta({ est_tokens: 'nope' })).toBeNull();
  });

  it('returns null when everything is zero', () => {
    expect(parseContextBudgetMeta({ est_tokens: {}, est_total_input: 0, tools_count: 0 })).toBeNull();
  });

  it('derives total from categories when est_total_input is missing', () => {
    const snap = parseContextBudgetMeta({ est_tokens: { history: 100, static_prefix: 50 } });
    expect(snap?.est_total_input).toBe(150);
  });

  it('drops non-positive / non-numeric category values and bad top_tools entries', () => {
    const snap = parseContextBudgetMeta({
      est_tokens: { history: 100, memory_l1: -5, other_dynamic: Number.NaN, skills: 'x' },
      est_total_input: 100,
      tools_count: 1,
      top_tools: [
        { name: '', est_tokens: 10 },
        { name: 'ok', est_tokens: 0 },
        'junk',
        { name: 'good', est_tokens: 42 },
      ],
    });
    expect(snap?.est_tokens).toEqual({ history: 100 });
    expect(snap?.top_tools).toEqual([{ name: 'good', est_tokens: 42 }]);
  });
});

describe('buildContextBudgetRows', () => {
  it('returns ordered non-zero rows following ROW_DEFS order', () => {
    const rows = buildContextBudgetRows({
      est_tokens: {
        history: 105900,
        static_prefix: 909,
        tools_schema: 10900,
        memory_l1: 300,
        memory_l4: 400,
        memory_composite: 206,
        knowledge_cue: 0,
      },
      est_total_input: 117715,
      tools_count: 5,
    });
    expect(rows.map((r) => r.key)).toEqual([
      CONTEXT_BUDGET_CATEGORY.staticPrefix,
      CONTEXT_BUDGET_CATEGORY.toolsSchema,
      CONTEXT_BUDGET_CATEGORY.memoryL1,
      CONTEXT_BUDGET_CATEGORY.memoryL4,
      CONTEXT_BUDGET_CATEGORY.memoryComposite,
      CONTEXT_BUDGET_CATEGORY.history,
    ]);
    expect(contextBudgetRowsTotal(rows)).toBe(909 + 10900 + 300 + 400 + 206 + 105900);
    // Memory L1/L4/composite must surface as separate rows (product decision).
    expect(rows.filter((r) => r.key.startsWith('memory_'))).toHaveLength(3);
  });

  it('returns empty for null/undefined or all-zero budgets', () => {
    expect(buildContextBudgetRows(null)).toEqual([]);
    expect(buildContextBudgetRows(undefined)).toEqual([]);
    expect(buildContextBudgetRows({ est_tokens: {}, est_total_input: 0, tools_count: 0 })).toEqual([]);
  });

  it('ignores unknown category keys', () => {
    const rows = buildContextBudgetRows({
      est_tokens: { some_future_category: 500 },
      est_total_input: 500,
      tools_count: 0,
    });
    expect(rows).toEqual([]);
  });
});
