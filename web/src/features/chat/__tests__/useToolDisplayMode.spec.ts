import { computed, ref } from 'vue';
import { describe, expect, it } from 'vitest';
import { useToolDisplayMode, TOOL_DISPLAY_THRESHOLD } from '../composables/useToolDisplayMode';
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

describe('useToolDisplayMode', () => {
  it('exports TOOL_DISPLAY_THRESHOLD = 2 as the single source of truth', () => {
    expect(TOOL_DISPLAY_THRESHOLD).toBe(2);
  });

  it("returns 'execution-card' for an empty list", () => {
    const events = computed(() => [] as readonly ToolUseEvent[]);
    const mode = useToolDisplayMode(events);
    expect(mode.value).toBe('execution-card');
  });

  it("returns 'execution-card' for a single event", () => {
    const events = computed(() => [makeEvent()] as readonly ToolUseEvent[]);
    const mode = useToolDisplayMode(events);
    expect(mode.value).toBe('execution-card');
  });

  it(`returns 'timeline' when events >= THRESHOLD (${2})`, () => {
    const events = computed(() => [
      makeEvent({ id: '1' }),
      makeEvent({ id: '2' }),
    ] as readonly ToolUseEvent[]);
    const mode = useToolDisplayMode(events);
    expect(mode.value).toBe('timeline');
  });

  it('returns \'timeline\' for large lists', () => {
    const events = computed(() =>
      Array.from({ length: 10 }, (_, i) => makeEvent({ id: `ev-${i}` })),
    );
    const mode = useToolDisplayMode(events);
    expect(mode.value).toBe('timeline');
  });

  it('updates reactively when the events ref changes', () => {
    // Use a `ref` so the computed re-evaluates when the source changes.
    const list1: ToolUseEvent[] = [makeEvent({ id: 'a' })];
    const list2: ToolUseEvent[] = [makeEvent({ id: 'a' }), makeEvent({ id: 'b' })];
    const active = ref<readonly ToolUseEvent[]>(list1);
    const events = computed(() => active.value);
    const mode = useToolDisplayMode(events);
    expect(mode.value).toBe('execution-card');
    active.value = list2;
    expect(mode.value).toBe('timeline');
  });
});
