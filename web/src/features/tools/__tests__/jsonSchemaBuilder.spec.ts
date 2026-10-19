import { describe, expect, it, vi } from 'vitest';
import { buildSchemaFromFields, configDiffSummary, configExtraKeys, parseSchemaFields } from '../jsonSchemaBuilder';
import type { SchemaFieldRow } from '../jsonSchemaBuilder';

function fieldRow(patch: Partial<SchemaFieldRow>): SchemaFieldRow {
  return { key: '', type: 'string', title: '', description: '', required: false, enumValues: '', ...patch };
}

describe('jsonSchemaBuilder', () => {
  it('round-trips simple parameter schema', () => {
    const json = JSON.stringify({
      type: 'object',
      properties: {
        query: { type: 'string', description: '搜索词' },
      },
      required: ['query'],
    });
    const rows = parseSchemaFields(json);
    expect(rows).toHaveLength(1);
    expect(rows[0].key).toBe('query');
    expect(rows[0].required).toBe(true);
    const rebuilt = buildSchemaFromFields(rows);
    expect(JSON.parse(rebuilt)).toEqual(JSON.parse(json));
  });

  it('detects extra config keys', () => {
    const extras = configExtraKeys('{"foo":1}', '{"type":"object","properties":{"bar":{"type":"string"}}}');
    expect(extras).toEqual(['foo']);
  });

  it('summarizes config diff', () => {
    const lines = configDiffSummary('{"timeout_sec":60}', '{"timeout_sec":30}');
    expect(lines.some((l) => l.includes('timeout_sec'))).toBe(true);
  });

  it('keeps integer enum values as numbers', () => {
    const rebuilt = buildSchemaFromFields([fieldRow({ key: 'count', type: 'integer', enumValues: '1, 2, 3' })]);
    expect(JSON.parse(rebuilt).properties.count.enum).toEqual([1, 2, 3]);
  });

  it('keeps string enum values as strings', () => {
    const rebuilt = buildSchemaFromFields([fieldRow({ key: 'mode', type: 'string', enumValues: 'a, b' })]);
    expect(JSON.parse(rebuilt).properties.mode.enum).toEqual(['a', 'b']);
  });

  it('keeps invalid number enum values as original strings', () => {
    const rebuilt = buildSchemaFromFields([fieldRow({ key: 'ratio', type: 'number', enumValues: '1.5, abc, 2' })]);
    expect(JSON.parse(rebuilt).properties.ratio.enum).toEqual([1.5, 'abc', 2]);
  });

  it('maps boolean enum values to booleans', () => {
    const rebuilt = buildSchemaFromFields([fieldRow({ key: 'flag', type: 'boolean', enumValues: 'true, false' })]);
    expect(JSON.parse(rebuilt).properties.flag.enum).toEqual([true, false]);
  });

  it('warns on duplicate keys and the later row wins', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    try {
      const rebuilt = buildSchemaFromFields([
        fieldRow({ key: 'q', type: 'string', title: 'first' }),
        fieldRow({ key: 'q', type: 'integer', title: 'second' }),
      ]);
      const props = JSON.parse(rebuilt).properties;
      expect(warn).toHaveBeenCalledOnce();
      expect(String(warn.mock.calls[0][0])).toContain('duplicate');
      expect(Object.keys(props)).toEqual(['q']);
      expect(props.q).toEqual({ type: 'integer', title: 'second' });
    } finally {
      warn.mockRestore();
    }
  });
});
