import { describe, expect, it } from 'vitest';
import { buildSchemaFromFields, configDiffSummary, configExtraKeys, parseSchemaFields } from '../jsonSchemaBuilder';

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
});
