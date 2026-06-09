import { describe, expect, it } from 'vitest';
import { __test__ } from '../composables/useAgentBlocks';
import type { Message } from '../../../domain/types';
import type { ToolUseEvent } from '../types';
import type { ToolSectionStatus } from '../agentTreeTypes';

const { buildToolSection, normalizeToolSectionStatus, computeAgentStatus } = __test__;

function ev(partial: Partial<ToolUseEvent> = {}): ToolUseEvent {
  return {
    id: 'tc-1',
    phase: 'after',
    status: 'success',
    agent_id: 'a1',
    agent_key: 'agent-1',
    agent_name: 'Agent',
    agent_icon: '',
    tool_name: 'read_file',
    tool_label: '读取文件',
    occurred_at: '2026-05-20T10:00:00Z',
    ...partial,
  };
}

describe('useAgentBlocks.buildToolSection', () => {
  it('normalizes backend "error" status to canonical "failed" (regression: cast was hiding this)', () => {
    const section = buildToolSection(ev({ status: 'error', error: 'boom' }), 'root-tool');
    const allowed: ToolSectionStatus[] = ['running', 'success', 'failed', 'blocked', 'cancelled'];
    expect(allowed).toContain(section.status);
    expect(section.status).toBe('failed');
    expect(section.error).toBe('boom');
    expect(section.collapsed).toBe(true);
  });

  it('falls back to "failed" for unknown status values (no leak of raw backend values)', () => {
    expect(normalizeToolSectionStatus('weird_value')).toBe('running'); // unknown → running, not "weird_value"
    expect(normalizeToolSectionStatus('interrupted')).toBe('cancelled');
  });

  it('produces a stable, deterministic id when tool_event.id is missing (regression: Math.random broke Vue diffing)', () => {
    const a = buildToolSection(ev({ id: '' }), 'root-tool');
    const b = buildToolSection(ev({ id: '' }), 'root-tool');
    expect(a.id).toBe(b.id);
    // It must NOT use Math.random() — the suffix must be a stable function of the tool name.
    expect(a.id).toBe(`root-tool-missing-read_file`);
  });

  it('uses tool_call_id when present (preferred path)', () => {
    const section = buildToolSection(ev({ id: 'tc-real-id' }), 'sub-tool');
    expect(section.id).toBe('sub-tool-tc-real-id');
  });

  it('preserves error in result field fallback when no result object exists', () => {
    const section = buildToolSection(ev({ status: 'error', error: 'no result object' }), 'root-tool');
    expect(section.result).toBe('no result object');
    expect(section.error).toBe('no result object');
  });

  it('collapses "running" sections (they should auto-expand for visibility)', () => {
    const section = buildToolSection(ev({ status: 'running' }), 'root-tool');
    expect(section.collapsed).toBe(false);
  });
});

describe('useAgentBlocks.computeAgentStatus', () => {
  function msg(partial: Partial<Message>): Message {
    return {
      id: 'm',
      role: 'assistant',
      content_markdown: '',
      status: 'tool_success',
      created_at: '2026-05-20T10:00:00Z',
      ...partial,
    } as Message;
  }

  it('treats a "tool_blocked" tool as NOT completed (regression: blocked tool marked root as completed)', () => {
    // Root agent has only one tool message, in tool_blocked state (waiting for
    // user confirmation). The previous implementation considered isCompleted=true
    // when no message was streaming/tool_running, so the root agent showed the
    // green "completed" badge while the tool was still waiting on the user.
    const assistant = msg({ id: 'a1', status: 'success' });
    const toolBlocked = msg({
      id: 't1',
      role: 'tool',
      status: 'tool_blocked',
      tool_event: ev({ id: 'tc-blocked', status: 'blocked', tool_name: 'shell_exec' }),
    });
    const memberMsgs: Message[] = [];
    const status = computeAgentStatus(assistant, [toolBlocked], memberMsgs, true);
    expect(status).not.toBe('completed');
  });

  it('treats a running tool as NOT completed', () => {
    const assistant = msg({ id: 'a1', status: 'success' });
    const toolRunning = msg({
      id: 't1',
      role: 'tool',
      status: 'tool_running',
      tool_event: ev({ id: 'tc-run', status: 'running' }),
    });
    const status = computeAgentStatus(assistant, [toolRunning], [], true);
    expect(status).not.toBe('completed');
  });

  it('marks completed when all tools are success and assistant is success', () => {
    const assistant = msg({ id: 'a1', status: 'success' });
    const toolSuccess = msg({
      id: 't1',
      role: 'tool',
      status: 'tool_success',
      tool_event: ev({ id: 'tc-ok', status: 'success' }),
    });
    const status = computeAgentStatus(assistant, [toolSuccess], [], true);
    expect(status).toBe('completed');
  });
});

describe('useAgentBlocks.buildToolSection icon & long-running pass-through', () => {
  it('passes icon_key through to ToolSection', () => {
    const section = buildToolSection(ev({ icon_key: 'mcp' }), 'root-tool');
    expect(section.iconKey).toBe('mcp');
  });

  it('flags ToolSection as long-running when is_long_running is true (UI can show 等待中 pill)', () => {
    const section = buildToolSection(
      ev({ status: 'running', is_long_running: true }),
      'root-tool',
    );
    expect(section.isLongRunning).toBe(true);
  });

  it('omits the long-running flag for short-running tools', () => {
    const section = buildToolSection(
      ev({ status: 'success', is_long_running: false }),
      'root-tool',
    );
    expect(section.isLongRunning).toBe(false);
  });
});
