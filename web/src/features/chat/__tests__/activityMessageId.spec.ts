import { describe, expect, it } from 'vitest';
import { activityMessageId } from '../lib/activityMessageId';

describe('activityMessageId', () => {
  it('uses act-{id} when a tool_call id is present (the canonical stable upsert key)', () => {
    expect(activityMessageId({ id: 'tc-1', agent_id: 'a', agent_key: 'k', tool_name: 'read_file' })).toBe('act-tc-1');
  });

  it('trims whitespace from the tool_call id', () => {
    expect(activityMessageId({ id: '  tc-1  ', agent_id: 'a', agent_key: 'k', tool_name: 'read_file' })).toBe(
      'act-tc-1',
    );
  });

  it('falls back to tool-{owner}-{tool_name} when no id is present (regression: must NOT be tool-{owner}-{""})', () => {
    expect(activityMessageId({ id: '', agent_id: 'a', agent_key: 'k', tool_name: 'read_file' })).toBe(
      'tool-a-read_file',
    );
  });

  it('prefers agent_id, then agent_key, then "agent"', () => {
    expect(activityMessageId({ id: '', agent_id: 'a1', agent_key: 'k1', tool_name: 't' })).toBe('tool-a1-t');
    expect(activityMessageId({ id: '', agent_id: '', agent_key: 'k1', tool_name: 't' })).toBe('tool-k1-t');
    expect(activityMessageId({ id: '', agent_id: '', agent_key: '', tool_name: 't' })).toBe('tool-agent-t');
  });

  it('is deterministic (regression: two calls with the same input produce the same key)', () => {
    const input = { id: 'tc-1', agent_id: 'a', agent_key: 'k', tool_name: 't' };
    expect(activityMessageId(input)).toBe(activityMessageId(input));
  });
});
