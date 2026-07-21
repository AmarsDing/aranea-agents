import { describe, expect, it } from 'vitest';
import { KNOWLEDGE_DOC_TABLE_COLUMNS, knowledgeDocColumns } from '../knowledgeUi';

describe('knowledgeUi doc table columns', () => {
  it('exposes updated_at column so users see last update time', () => {
    const names = KNOWLEDGE_DOC_TABLE_COLUMNS.map((c) => c.name);
    expect(names).toContain('updated_at');
  });

  it('keeps created_at before updated_at', () => {
    const names = KNOWLEDGE_DOC_TABLE_COLUMNS.map((c) => c.name);
    expect(names.indexOf('created_at')).toBeLessThan(names.indexOf('updated_at'));
  });

  it('deprecated alias stays in sync', () => {
    expect(knowledgeDocColumns).toBe(KNOWLEDGE_DOC_TABLE_COLUMNS);
  });
});
