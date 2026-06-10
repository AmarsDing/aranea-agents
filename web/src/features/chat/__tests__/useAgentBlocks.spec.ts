import { describe, expect, it } from 'vitest';
import { computed, ref } from 'vue';
import { __test__, useAgentBlocks } from '../composables/useAgentBlocks';
import type { Message } from '../../../domain/types';
import type { ToolUseEvent } from '../types';
import type { ToolSectionStatus } from '../agentTreeTypes';
import type { Envelope } from '../../../realtime/envelope';

const { buildToolSection, normalizeToolSectionStatus, computeAgentStatus } = __test__;

function ev(partial: Partial<ToolUseEvent> = {}): ToolUseEvent {
  return {
    id: 'tc-1',
    phase: 'after',
    status: 'success',
    agent_id: 'a1',
    agent_key: 'agent-1',
    agent_name: 'Agent',
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

describe('useAgentBlocks single-agent + progress envelopes', () => {
  function userMsg(partial: Partial<Message> = {}): Message {
    return {
      id: 'u1',
      role: 'user',
      content_markdown: '你好',
      created_at: '2026-06-10T10:00:00.000Z',
      status: 'ok',
      ...partial,
    } as Message;
  }

  function assistantMsg(partial: Partial<Message> = {}): Message {
    return {
      id: 'a1',
      role: 'assistant',
      content_markdown: '正在思考…',
      created_at: '2026-06-10T10:00:05.000Z',
      status: 'streaming',
      ...partial,
    } as Message;
  }

  function progressEnv(partial: {
    id?: string;
    stepId?: string;
    phase?: 'start' | 'done' | 'error';
    message?: string;
    category?: 'orchestration' | 'team' | 'tool' | 'thinking';
    timestamp?: string;
    durationMs?: number;
  }): Envelope {
    return {
      id: partial.id ?? 'env-1',
      type: 'execution_progress',
      author: 'system',
      session_id: 's1',
      timestamp: partial.timestamp ?? '2026-06-10T10:00:01.000Z',
      version: 1,
      metadata: {
        step_id: partial.stepId ?? 'chat.llm.invoke',
        phase: partial.phase ?? 'start',
        message: partial.message ?? '正在调用语言模型',
        category: partial.category ?? 'orchestration',
        ...(typeof partial.durationMs === 'number' ? { duration_ms: partial.durationMs } : {}),
      },
    } as Envelope;
  }

  it('produces a root AgentBlock for a single-agent session (P0: team gate removed)', () => {
    const messages = ref<Message[]>([userMsg(), assistantMsg()]);
    const { agentBlocks } = useAgentBlocks({ messages: computed(() => messages.value) });
    expect(agentBlocks.value.length).toBe(1);
    expect(agentBlocks.value[0]?.agentKey).toBeTruthy();
  });

  it('merges execution_progress envelopes into the timeline as progress entries', () => {
    const messages = ref<Message[]>([userMsg(), assistantMsg()]);
    const progress = ref<readonly Envelope[]>([
      progressEnv({ phase: 'start', message: '正在调用语言模型' }),
      progressEnv({
        id: 'env-2',
        phase: 'done',
        message: '语言模型已返回',
        timestamp: '2026-06-10T10:00:03.000Z',
        durationMs: 2000,
      }),
    ]);
    const { agentBlocks } = useAgentBlocks({
      messages: computed(() => messages.value),
      progressEnvelopes: computed(() => progress.value),
    });
    const timeline = agentBlocks.value[0]?.timeline ?? [];
    const progressEntries = timeline.filter((e) => e.kind === 'progress');
    expect(progressEntries.length).toBe(1); // same step_id merged
    const section = (progressEntries[0] as { section: { status: string; message: string; durationMs: number | null } })
      .section;
    expect(section.status).toBe('done');
    expect(section.message).toBe('语言模型已返回');
    expect(section.durationMs).toBe(2000);
  });

  it('emits no progress entries when progressEnvelopes is not provided', () => {
    const messages = ref<Message[]>([userMsg(), assistantMsg()]);
    const { agentBlocks } = useAgentBlocks({ messages: computed(() => messages.value) });
    const timeline = agentBlocks.value[0]?.timeline ?? [];
    expect(timeline.some((e) => e.kind === 'progress')).toBe(false);
  });
});

describe('useAgentBlocks multi-round reply entries (P1 reply-chronological)', () => {
  function userMsg(partial: Partial<Message> = {}): Message {
    return {
      id: 'u1',
      role: 'user',
      content_markdown: '你好',
      created_at: '2026-06-10T10:00:00.000Z',
      status: 'ok',
      ...partial,
    } as Message;
  }

  function assistantMsg(partial: Partial<Message> = {}): Message {
    return {
      id: 'a-default',
      role: 'assistant',
      content_markdown: '默认回复',
      created_at: '2026-06-10T10:00:05.000Z',
      status: 'success',
      ...partial,
    } as Message;
  }

  it('emits one reply entry per assistant message in chronological order (non-ReAct mode)', () => {
    // Three round-trips: each assistant message becomes its own reply entry
    // in the timeline at its own chronological position, NOT collapsed into
    // a single "result" field with paragraph-splitting.
    const messages = ref<Message[]>([
      userMsg(),
      assistantMsg({ id: 'a1', content_markdown: '第一轮回复', created_at: '2026-06-10T10:00:01.000Z' }),
      assistantMsg({ id: 'a2', content_markdown: '第二轮回复', created_at: '2026-06-10T10:00:03.000Z' }),
      assistantMsg({ id: 'a3', content_markdown: '第三轮回复', created_at: '2026-06-10T10:00:05.000Z' }),
    ]);
    const { agentBlocks } = useAgentBlocks({ messages: computed(() => messages.value) });
    const timeline = agentBlocks.value[0]?.timeline ?? [];
    const replies = timeline.filter((e) => e.kind === 'reply');
    expect(replies.length).toBe(3);
    // Each reply has a distinct stable id tied to its assistant message id,
    // so Vue can diff them and the user sees three independent UI cards.
    const replyIds = replies.map((e) => (e as { section: { id: string } }).section.id);
    expect(new Set(replyIds).size).toBe(3);
    expect(replyIds).toContain('root-reply-a1');
    expect(replyIds).toContain('root-reply-a2');
    expect(replyIds).toContain('root-reply-a3');
    // Each reply entry carries its own content (no collapse into one field).
    const replyContents = replies.map((e) => (e as { section: { content: string } }).section.content);
    expect(replyContents).toEqual(['第一轮回复', '第二轮回复', '第三轮回复']);
  });

  it('emits one reply entry per assistant message in ReAct mode when FINAL_ANSWER tag is present', () => {
    // Two ReAct turns, each with a /*FINAL_ANSWER*/ tag → two separate
    // reply entries in the timeline. This is the core "multi-round reply →
    // multiple UI components" requirement.
    const messages = ref<Message[]>([
      userMsg(),
      assistantMsg({
        id: 'react-1',
        content_markdown: '/*REASONING*/先思考一下\n/*FINAL_ANSWER*/回答一',
        created_at: '2026-06-10T10:00:01.000Z',
      }),
      assistantMsg({
        id: 'react-2',
        content_markdown: '/*REASONING*/再想想\n/*FINAL_ANSWER*/回答二',
        created_at: '2026-06-10T10:00:03.000Z',
      }),
    ]);
    const { agentBlocks } = useAgentBlocks({
      messages: computed(() => messages.value),
      plannerKind: computed(() => 'react'),
    });
    const timeline = agentBlocks.value[0]?.timeline ?? [];
    const replies = timeline.filter((e) => e.kind === 'reply');
    expect(replies.length).toBe(2);
    const replyContents = replies.map((e) => (e as { section: { content: string } }).section.content);
    expect(replyContents).toEqual(['回答一', '回答二']);
  });

  it('emits NO reply entry for ReAct assistant messages that lack the FINAL_ANSWER tag', () => {
    // ReAct mode without a final-answer tag should not echo the last
    // step body as a duplicate reply (regression: the legacy
    // `presentation.reactSteps.finalAnswer` fallback would render the
    // last step as both a thinking entry AND a reply entry).
    const messages = ref<Message[]>([
      userMsg(),
      assistantMsg({
        id: 'react-no-final',
        content_markdown: '/*REASONING*/只是推理，没有最终答案',
        created_at: '2026-06-10T10:00:01.000Z',
      }),
    ]);
    const { agentBlocks } = useAgentBlocks({
      messages: computed(() => messages.value),
      plannerKind: computed(() => 'react'),
    });
    const timeline = agentBlocks.value[0]?.timeline ?? [];
    const replies = timeline.filter((e) => e.kind === 'reply');
    expect(replies.length).toBe(0);
  });

  it('skips a reply entry that would duplicate the immediately preceding thinking entry', () => {
    // Non-ReAct mode where the assistant's content_markdown exactly equals
    // its reasoning_markdown → the de-dup guard prevents emitting an
    // identical reply right after the thinking entry.
    const messages = ref<Message[]>([
      userMsg(),
      assistantMsg({
        id: 'dedup',
        content_markdown: '相同的文字',
        reasoning_markdown: '相同的文字',
        created_at: '2026-06-10T10:00:01.000Z',
      }),
    ]);
    const { agentBlocks } = useAgentBlocks({ messages: computed(() => messages.value) });
    const timeline = agentBlocks.value[0]?.timeline ?? [];
    const replies = timeline.filter((e) => e.kind === 'reply');
    expect(replies.length).toBe(0);
  });

  it('preserves chronological ordering: thinking → reply → thinking → reply', () => {
    // Each round has its own thinking + reply. They must appear in
    // chronological order in the timeline so the UI can render them
    // as separate cards in the order they occurred.
    const messages = ref<Message[]>([
      userMsg(),
      assistantMsg({
        id: 'r1',
        content_markdown: '回复 1',
        reasoning_markdown: '思考 1',
        created_at: '2026-06-10T10:00:01.000Z',
      }),
      assistantMsg({
        id: 'r2',
        content_markdown: '回复 2',
        reasoning_markdown: '思考 2',
        created_at: '2026-06-10T10:00:03.000Z',
      }),
    ]);
    const { agentBlocks } = useAgentBlocks({ messages: computed(() => messages.value) });
    const timeline = agentBlocks.value[0]?.timeline ?? [];
    // Walk the timeline and check the kind sequence.
    const kinds = timeline.map((e) => e.kind);
    // First thinking then reply, then thinking then reply.
    expect(kinds).toEqual(['thinking', 'reply', 'thinking', 'reply']);
  });

  it('result field falls back to the LAST reply entry content (backward compat)', () => {
    // The legacy `block.result` field is no longer used to drive 1:1 pairing,
    // but is preserved as the content of the last reply entry for callers
    // that still read it (e.g. accessibility / collapsed summaries).
    const messages = ref<Message[]>([
      userMsg(),
      assistantMsg({ id: 'r1', content_markdown: '第一轮', created_at: '2026-06-10T10:00:01.000Z' }),
      assistantMsg({ id: 'r2', content_markdown: '第二轮', created_at: '2026-06-10T10:00:03.000Z' }),
    ]);
    const { agentBlocks } = useAgentBlocks({ messages: computed(() => messages.value) });
    expect(agentBlocks.value[0]?.result).toBe('第二轮');
  });
});
