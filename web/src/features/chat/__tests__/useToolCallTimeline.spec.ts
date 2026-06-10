import { computed } from 'vue';
import { describe, expect, it } from 'vitest';
import { useToolCallTimeline, buildTimelineNode } from '../composables/useToolCallTimeline';
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
    occurred_at: '2026-01-01T10:30:45Z',
    ...overrides,
  };
}

describe('useToolCallTimeline', () => {
  it('sorts events by occurred_at ascending', () => {
    const ev1 = makeEvent({ id: '1', occurred_at: '2026-01-01T10:00:00Z' });
    const ev2 = makeEvent({ id: '2', occurred_at: '2026-01-01T09:00:00Z' });
    const ev3 = makeEvent({ id: '3', occurred_at: '2026-01-01T11:00:00Z' });

    const events = computed(() => [ev1, ev2, ev3]);
    const { sortedEvents } = useToolCallTimeline(events);

    expect(sortedEvents.value[0].id).toBe('2');
    expect(sortedEvents.value[1].id).toBe('1');
    expect(sortedEvents.value[2].id).toBe('3');
  });

  it('falls back to id sort when occurred_at is the same', () => {
    const ev1 = makeEvent({ id: 'b', occurred_at: '2026-01-01T10:00:00Z' });
    const ev2 = makeEvent({ id: 'a', occurred_at: '2026-01-01T10:00:00Z' });

    const events = computed(() => [ev1, ev2]);
    const { sortedEvents } = useToolCallTimeline(events);

    expect(sortedEvents.value[0].id).toBe('a');
    expect(sortedEvents.value[1].id).toBe('b');
  });

  it('handles empty events list', () => {
    const events = computed(() => []);
    const { sortedEvents } = useToolCallTimeline(events);
    expect(sortedEvents.value).toHaveLength(0);
  });
});

describe('buildTimelineNode', () => {
  it('builds a node with correct timestamp format', () => {
    const ev = makeEvent({ occurred_at: '2026-01-01T14:25:37Z' });
    const node = buildTimelineNode(ev);
    // Timestamp is in local time, just verify the format HH:MM:SS
    expect(node.timestamp).toMatch(/^\d{2}:\d{2}:\d{2}$/);
  });

  it('marks stuck tools with danger status point', () => {
    const ev = makeEvent({ error_code: 'tool_timeout' });
    const node = buildTimelineNode(ev);
    expect(node.isStuck).toBe(true);
    expect(node.statusPoint.color).toBe('var(--color-danger)');
  });

  it('maps success status to correct color', () => {
    const ev = makeEvent({ status: 'success' });
    const node = buildTimelineNode(ev);
    expect(node.statusPoint.color).toBe('var(--color-success)');
    expect(node.statusPoint.animated).toBe(false);
  });

  it('maps running status to animated warning', () => {
    const ev = makeEvent({ status: 'running' });
    const node = buildTimelineNode(ev);
    expect(node.statusPoint.color).toBe('var(--color-warning)');
    expect(node.statusPoint.animated).toBe(true);
  });

  it('maps failed status to danger color', () => {
    const ev = makeEvent({ status: 'failed' });
    const node = buildTimelineNode(ev);
    expect(node.statusPoint.color).toBe('var(--color-danger)');
  });

  it('includes error text when present', () => {
    const ev = makeEvent({ error: 'File not found' });
    const node = buildTimelineNode(ev);
    expect(node.errorText).toBe('File not found');
  });

  it('omits error text when empty', () => {
    const ev = makeEvent({ error: '' });
    const node = buildTimelineNode(ev);
    expect(node.errorText).toBeUndefined();
  });

  it('includes args preview when arguments present', () => {
    const ev = makeEvent({ arguments: { file: 'test.ts' } });
    const node = buildTimelineNode(ev);
    expect(node.argsPreview).toBeDefined();
    expect(node.argsPreview).toContain('file');
  });

  it('includes result preview when result present', () => {
    const ev = makeEvent({ result: { content: 'hello' } });
    const node = buildTimelineNode(ev);
    expect(node.resultPreview).toBeDefined();
    expect(node.resultPreview).toContain('content');
  });

  it('includes duration label when duration_ms present', () => {
    const ev = makeEvent({ duration_ms: 1500 });
    const node = buildTimelineNode(ev);
    expect(node.durationLabel).toBeTruthy();
  });
});
