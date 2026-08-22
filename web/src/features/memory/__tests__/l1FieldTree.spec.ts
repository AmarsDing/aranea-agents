import { describe, expect, it } from 'vitest';
import { buildL1FieldTree, taskBudgetRatio } from '../l1FieldTree';
import type { L1Field } from '../types';

function field(path: string, extras: Partial<L1Field> = {}): L1Field {
  return {
    id: path,
    task_id: 't1',
    session_id: 's1',
    agent_id: 'a1',
    field_path: path,
    field_kind: 'string',
    visibility: 'internal',
    pin_to_prompt: false,
    is_required: false,
    value_text: '',
    value_json: '',
    value_ref: '',
    preview: extras.preview ?? path,
    token_estimate: extras.token_estimate ?? 4,
    source: extras.source ?? 'tool',
    source_ref: '',
    ttl_seconds: 0,
    expires_at: '',
    revision: extras.revision ?? 1,
    last_read_at: '',
    read_count: 0,
    metadata_json: '',
    created_at: '',
    updated_at: '',
    ...extras,
  };
}

describe('buildL1FieldTree', () => {
  it('nests dotted paths and keeps leaf metadata', () => {
    const tree = buildL1FieldTree([
      field('task_goal', { preview: '写周报', pin_to_prompt: true, revision: 2 }),
      field('subtasks.draft', { preview: '提纲', token_estimate: 8 }),
      field('subtasks.review'),
    ]);
    expect(tree.map((n) => n.label)).toEqual(['subtasks', 'task_goal']);
    expect(tree[1].preview).toBe('写周报');
    expect(tree[1].pinned).toBe(true);
    expect(tree[1].revision).toBe(2);
    expect(tree[0].children?.map((c) => c.label)).toEqual(['draft', 'review']);
    expect(tree[0].children?.[0].tokens).toBe(8);
  });

  it('returns empty for missing fields', () => {
    expect(buildL1FieldTree(null)).toEqual([]);
    expect(buildL1FieldTree([])).toEqual([]);
  });
});

describe('taskBudgetRatio', () => {
  it('clamps used/budget into 0..1', () => {
    expect(taskBudgetRatio(30, 100)).toBe(0.3);
    expect(taskBudgetRatio(200, 100)).toBe(1);
    expect(taskBudgetRatio(10, 0)).toBe(0);
  });
});
