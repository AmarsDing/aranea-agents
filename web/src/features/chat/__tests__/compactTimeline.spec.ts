import { describe, expect, it } from 'vitest';
import { buildCompactNodes, compactNodeKey } from '../compactTimeline';
import type { ToolUseEvent } from '../types';

function makeToolEvent(id: string, overrides: Partial<ToolUseEvent> = {}): ToolUseEvent {
  return {
    id,
    phase: 'after',
    status: 'success',
    agent_id: 'a1',
    agent_key: 'agent',
    agent_name: 'Agent',
    tool_name: 'get_sales',
    tool_label: 'get_sales',
    display_label: 'get_sales',
    arguments: {},
    result: {},
    occurred_at: '2026-06-10T10:42:02Z',
    ...overrides,
  } as ToolUseEvent;
}

describe('buildCompactNodes', () => {
  it('returns empty array when all inputs are empty', () => {
    const nodes = buildCompactNodes({
      reasoning: '',
      bodyMarkdown: '',
      toolEvents: [],
      messageId: 'm1',
      isStreaming: false,
    });
    expect(nodes).toEqual([]);
  });

  it('returns single reply when only body exists', () => {
    const nodes = buildCompactNodes({
      reasoning: '',
      bodyMarkdown: 'Final answer here.',
      toolEvents: [],
      messageId: 'm1',
      isStreaming: false,
    });
    expect(nodes).toHaveLength(1);
    expect(nodes[0]).toMatchObject({ kind: 'reply', text: 'Final answer here.', messageId: 'm1', streaming: false, status: 'ok' });
  });

  it('returns [thinking, reply] when no tool exists', () => {
    const nodes = buildCompactNodes({
      reasoning: 'User wants trend analysis.',
      bodyMarkdown: 'Here is the result.',
      toolEvents: [],
      messageId: 'm1',
      isStreaming: false,
    });
    expect(nodes).toHaveLength(2);
    expect(nodes[0]).toMatchObject({ kind: 'thinking', text: 'User wants trend analysis.' });
    expect(nodes[1]).toMatchObject({ kind: 'reply', text: 'Here is the result.' });
  });

  it('returns [thinking, tool, reply] with single tool', () => {
    const tool = makeToolEvent('tc-1');
    const nodes = buildCompactNodes({
      reasoning: 'I need to query sales.',
      bodyMarkdown: 'Sales are trending up.',
      toolEvents: [tool],
      messageId: 'm1',
      isStreaming: false,
    });
    expect(nodes).toHaveLength(3);
    expect(nodes[0]?.kind).toBe('thinking');
    expect(nodes[1]).toMatchObject({ kind: 'tool', event: tool });
    expect(nodes[2]?.kind).toBe('reply');
  });

  it('preserves tool order and does not distribute tools across rounds', () => {
    const t1 = makeToolEvent('tc-1', { tool_name: 'get_sales' });
    const t2 = makeToolEvent('tc-2', { tool_name: 'get_sales' });
    const t3 = makeToolEvent('tc-3', { tool_name: 'analyze' });
    const nodes = buildCompactNodes({
      reasoning: 'Need data + analysis.',
      bodyMarkdown: 'Done.',
      toolEvents: [t1, t2, t3],
      messageId: 'm1',
      isStreaming: false,
    });
    expect(nodes).toHaveLength(5);
    expect(nodes[0]?.kind).toBe('thinking');
    expect(nodes[1]).toMatchObject({ kind: 'tool', event: t1 });
    expect(nodes[2]).toMatchObject({ kind: 'tool', event: t2 });
    expect(nodes[3]).toMatchObject({ kind: 'tool', event: t3 });
    expect(nodes[4]?.kind).toBe('reply');
  });

  it('does not split reasoning by paragraph breaks (preserves original text)', () => {
    const reasoning = 'Para 1.\n\nPara 2 with list:\n- item a\n- item b\n\nPara 3.';
    const nodes = buildCompactNodes({
      reasoning,
      bodyMarkdown: '',
      toolEvents: [],
      messageId: 'm1',
      isStreaming: false,
    });
    expect(nodes).toHaveLength(1);
    expect(nodes[0]).toMatchObject({ kind: 'thinking' });
    if (nodes[0]?.kind === 'thinking') {
      // 整段保留，包含 \n\n，不切分
      expect(nodes[0].text).toBe(reasoning.trim());
      expect(nodes[0].text).toContain('\n\n');
    }
  });

  it('marks reply as streaming when isStreaming=true', () => {
    const nodes = buildCompactNodes({
      reasoning: '',
      bodyMarkdown: 'Generating...',
      toolEvents: [],
      messageId: 'm1',
      isStreaming: true,
    });
    expect(nodes[0]).toMatchObject({ kind: 'reply', streaming: true, status: 'streaming' });
  });

  it('marks reply as failed when messageStatus=failed', () => {
    const nodes = buildCompactNodes({
      reasoning: '',
      bodyMarkdown: 'Partial output',
      toolEvents: [],
      messageId: 'm1',
      isStreaming: false,
      messageStatus: 'failed',
    });
    expect(nodes[0]).toMatchObject({ kind: 'reply', status: 'failed', streaming: false });
  });

  it('marks reply as cancelled when messageStatus=cancelled', () => {
    const nodes = buildCompactNodes({
      reasoning: '',
      bodyMarkdown: 'Stopped',
      toolEvents: [],
      messageId: 'm1',
      isStreaming: false,
      messageStatus: 'cancelled',
    });
    expect(nodes[0]).toMatchObject({ kind: 'reply', status: 'cancelled' });
  });

  it('returns only tools when reasoning/body are both empty', () => {
    const t1 = makeToolEvent('tc-1');
    const t2 = makeToolEvent('tc-2');
    const nodes = buildCompactNodes({
      reasoning: '',
      bodyMarkdown: '',
      toolEvents: [t1, t2],
      messageId: 'm1',
      isStreaming: false,
    });
    expect(nodes).toHaveLength(2);
    expect(nodes.every((n) => n.kind === 'tool')).toBe(true);
  });

  it('trims whitespace from reasoning and body', () => {
    const nodes = buildCompactNodes({
      reasoning: '   \n\n  Thinking  \n\n   ',
      bodyMarkdown: '   \n\n  Reply  \n\n   ',
      toolEvents: [],
      messageId: 'm1',
      isStreaming: false,
    });
    expect(nodes).toHaveLength(2);
    if (nodes[0]?.kind === 'thinking') expect(nodes[0].text).toBe('Thinking');
    if (nodes[1]?.kind === 'reply') expect(nodes[1].text).toBe('Reply');
  });
});

describe('compactNodeKey', () => {
  it('returns stable key for thinking', () => {
    const node = { kind: 'thinking' as const, text: 'x', messageId: 'm1', streaming: false, durationMs: null };
    expect(compactNodeKey(node)).toBe('m1-thinking');
  });

  it('returns unique key per tool event id', () => {
    const n1 = { kind: 'tool' as const, event: makeToolEvent('tc-1') };
    const n2 = { kind: 'tool' as const, event: makeToolEvent('tc-2') };
    expect(compactNodeKey(n1)).not.toBe(compactNodeKey(n2));
  });

  it('returns stable key for reply', () => {
    const node = { kind: 'reply' as const, text: 'x', messageId: 'm1', streaming: false, status: 'ok' as const };
    expect(compactNodeKey(node)).toBe('m1-reply');
  });
});
