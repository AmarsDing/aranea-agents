import { describe, expect, it } from 'vitest';
import { isStuckTool } from '../lib/isStuckTool';
import type { ToolUseEvent } from '../types';

function makeEvent(overrides: Partial<ToolUseEvent> = {}): ToolUseEvent {
  return {
    id: 'ev-1',
    phase: 'after',
    status: 'success',
    agent_id: 'agent-1',
    agent_key: 'agent-1',
    agent_name: 'Agent',
    tool_name: 'read_file',
    tool_label: '读取文件',
    occurred_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

describe('isStuckTool', () => {
  it('returns true when error_code is tool_timeout', () => {
    const ev = makeEvent({ error_code: 'tool_timeout' });
    expect(isStuckTool(ev)).toBe(true);
  });

  it('returns false when error_code is other value', () => {
    const ev = makeEvent({ error_code: 'permission_denied' });
    expect(isStuckTool(ev)).toBe(false);
  });

  it('returns false when error_code is undefined', () => {
    const ev = makeEvent();
    expect(isStuckTool(ev)).toBe(false);
  });

  it('returns false for empty string error_code', () => {
    const ev = makeEvent({ error_code: '' });
    expect(isStuckTool(ev)).toBe(false);
  });
});
