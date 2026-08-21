import { describe, it, expect } from 'vitest';
import { generateSummaryFallback } from '../executionCardHelpers';
import type { ToolUseEvent } from '../types';

function event(partial: Partial<ToolUseEvent>): ToolUseEvent {
  return {
    id: 'e1',
    phase: 'before',
    status: 'running',
    agent_id: 'a',
    agent_key: '__spirit__',
    agent_name: 'Spirit',
    tool_name: '',
    tool_label: '',
    occurred_at: '2026-08-22T00:00:00Z',
    ...partial,
  };
}

describe('generateSummaryFallback', () => {
  it('plan_and_execute shows planning copy while the tool is running', () => {
    expect(generateSummaryFallback(event({ tool_name: 'plan_and_execute' }))).toBe('正在规划并执行…');
  });

  it('file_read uses the filename', () => {
    expect(
      generateSummaryFallback(event({ tool_name: 'file_read', arguments: { file_name: 'a.go' } })),
    ).toBe('读取 a.go');
  });
});
