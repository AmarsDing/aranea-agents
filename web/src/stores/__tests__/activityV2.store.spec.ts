// web/src/stores/__tests__/activityV2.store.spec.ts
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { flushPromises } from '@vue/test-utils';

// P2-07: mock the v2Api so fetchSessionHistory can be tested in isolation.
vi.mock('../../features/session/v2Api', () => ({
  listTasksV2: vi.fn(),
  listTurnsV2: vi.fn(),
  listStepsV2: vi.fn(),
  listTeamStagesV2: vi.fn(),
  listTeamRunsV2: vi.fn(),
  listMemberSessionsV2: vi.fn(),
  listOrphanMemberSessionsV2: vi.fn(),
  listPlanBoardsV2: vi.fn(),
  listPlanStepsV2: vi.fn(),
  listGraphStagesV2: vi.fn(),
  listGraphNodesV2: vi.fn(),
}));

import {
  listTasksV2,
  listTurnsV2,
  listStepsV2,
  listTeamStagesV2,
  listPlanBoardsV2,
  listPlanStepsV2,
  listGraphStagesV2,
  listOrphanMemberSessionsV2,
} from '../../features/session/v2Api';
import { useChatActivityStore } from '../chat/activityV2Store';
import { clearAllStepLiveText } from '../../features/chat/stepLiveTextCache';
import type { Task, Step } from '../../features/chat/v2Types';

function makeTask(over: Partial<Task> = {}): Task {
  return {
    ID: 't1',
    SessionID: 's1',
    UserMessage: 'hi',
    Status: 'running',
    Seq: 1,
    Version: 1,
    CreatedAt: '',
    UpdatedAt: '',
    CompletedAt: null,
    ...over,
  };
}

function makeNoticeStep(over: Partial<Step> = {}): Step {
  return {
    ID: 'n1',
    TurnID: 'turn-1',
    TaskID: 'task-1',
    SessionID: 'sess-1',
    SpiritSessionID: 'spirit-1',
    Kind: 'notice',
    AuthorAgentKey: 'a1',
    Seq: 1,
    Version: 1,
    Content: '',
    Reasoning: '',
    ToolName: '',
    ToolCallID: '',
    ToolArgs: null,
    ToolResult: null,
    ToolDurationMs: 0,
    ToolErrorCode: '',
    NoticeType: 'memory_recalled',
    Status: 'completed',
    IsFinal: false,
    StartedAt: '',
    CompletedAt: null,
    ...over,
  };
}

describe('useChatActivityStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    // P3-resume: live-text 缓存是模块级单例，用例间硬编码 stepId（'s1' 等）
    // 复用会串缓存——每个用例前全量清理。
    clearAllStepLiveText();
  });

  it('starts empty', () => {
    const s = useChatActivityStore();
    expect(s.tasks.size).toBe(0);
    expect(s.turns.size).toBe(0);
    expect(s.steps.size).toBe(0);
  });

  it('upsertTask adds a task', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't1' }));
    expect(s.tasks.get('t1')?.UserMessage).toBe('hi');
  });

  it('upsertTask replaces with higher version', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't1', Version: 1, Status: 'running' }));
    s.upsertTask(makeTask({ ID: 't1', Version: 2, Status: 'completed' }));
    expect(s.tasks.get('t1')?.Status).toBe('completed');
  });

  it('upsertTask ignores lower version', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't1', Version: 2, Status: 'completed' }));
    s.upsertTask(makeTask({ ID: 't1', Version: 1, Status: 'running' }));
    expect(s.tasks.get('t1')?.Status).toBe('completed');
  });

  // 2026-07-25 回归：Go 零值 time.Time 序列化为 "0001-01-01T00:00:00Z"（truthy），
  // 不能覆盖已有的 CreatedAt/UpdatedAt（否则任务创建时间显示为 01-01 08:05）。
  it('upsertTask treats Go zero-time as unset (preserves existing CreatedAt/UpdatedAt)', () => {
    const s = useChatActivityStore();
    s.upsertTask(
      makeTask({
        ID: 't1',
        Version: 1,
        Status: 'running',
        CreatedAt: '2026-07-25T06:13:00Z',
        UpdatedAt: '2026-07-25T06:13:00Z',
      }),
    );
    // synthesis turn 的 task.completed 最小载荷（后端 terminalTask 修复前的形态）
    s.upsertTask(
      makeTask({
        ID: 't1',
        Version: 2,
        Status: 'completed',
        CreatedAt: '0001-01-01T00:00:00Z',
        UpdatedAt: '0001-01-01T00:00:00Z',
        CompletedAt: '2026-07-25T06:15:00Z',
      }),
    );
    const task = s.tasks.get('t1');
    expect(task?.Status).toBe('completed');
    expect(task?.CreatedAt).toBe('2026-07-25T06:13:00Z');
    expect(task?.UpdatedAt).toBe('2026-07-25T06:13:00Z');
    expect(task?.CompletedAt).toBe('2026-07-25T06:15:00Z');
  });

  it('upsertStep merges streaming content', () => {
    const s = useChatActivityStore();
    const step: Step = {
      ID: 's1',
      TurnID: 't1',
      TaskID: 'tk1',
      SessionID: 's1',
      SpiritSessionID: 's1',
      Kind: 'reply',
      AuthorAgentKey: 'a1',
      Seq: 1,
      Version: 1,
      Content: '',
      Reasoning: '',
      ToolName: '',
      ToolCallID: '',
      ToolArgs: null,
      ToolResult: null,
      ToolDurationMs: 0,
      ToolErrorCode: '',
      Status: 'running',
      IsFinal: false,
      StartedAt: '',
      CompletedAt: null,
    };
    s.upsertStep(step);
    s.appendStepDelta('s1', 'content', 'hello ');
    s.appendStepDelta('s1', 'content', 'world');
    expect(s.steps.get('s1')?.Content).toBe('hello world');
  });

  // P3 验证：DeltaSeq 序号去重不得误杀合法的重复 chunk（叠词/重复标点/缩进）。
  // LLM 流式输出连续两个完全相同的 delta 是合法输入，序号递增时必须全部追加。
  it('appendStepDelta applies consecutive identical chunks (no false dedup)', () => {
    const s = useChatActivityStore();
    const step: Step = {
      ID: 's1',
      TurnID: 't1',
      TaskID: 'tk1',
      SessionID: 's1',
      SpiritSessionID: 's1',
      Kind: 'reply',
      AuthorAgentKey: 'a1',
      Seq: 1,
      Version: 1,
      Content: '',
      Reasoning: '',
      ToolName: '',
      ToolCallID: '',
      ToolArgs: null,
      ToolResult: null,
      ToolDurationMs: 0,
      ToolErrorCode: '',
      Status: 'running',
      IsFinal: false,
      StartedAt: '',
      CompletedAt: null,
    };
    s.upsertStep(step);
    // 带递增序号（生产路径：Sequencer 分配会话级单调 DeltaSeq）。
    s.appendStepDelta('s1', 'content', '哈', 1);
    s.appendStepDelta('s1', 'content', '哈', 2);
    s.appendStepDelta('s1', 'content', '哈', 3);
    expect(s.steps.get('s1')?.Content).toBe('哈哈哈');
    s.appendStepDelta('s1', 'reasoning', '  ', 4);
    s.appendStepDelta('s1', 'reasoning', '  ', 5);
    expect(s.steps.get('s1')?.Reasoning).toBe('    ');
    // 无序号（legacy/边界生产者）：总是应用，不做任何去重。
    s.appendStepDelta('s1', 'content', '哈');
    s.appendStepDelta('s1', 'content', '哈');
    expect(s.steps.get('s1')?.Content).toBe('哈哈哈哈哈');
  });

  // P3 验证：重发/乱序的 delta（DeltaSeq <= lastSeen）必须丢弃，
  // 否则 WS 重投递会产生重复字。
  it('appendStepDelta drops redelivered or out-of-order deltas by DeltaSeq', () => {
    const s = useChatActivityStore();
    const step: Step = {
      ID: 's1',
      TurnID: 't1',
      TaskID: 'tk1',
      SessionID: 's1',
      SpiritSessionID: 's1',
      Kind: 'reply',
      AuthorAgentKey: 'a1',
      Seq: 1,
      Version: 1,
      Content: '',
      Reasoning: '',
      ToolName: '',
      ToolCallID: '',
      ToolArgs: null,
      ToolResult: null,
      ToolDurationMs: 0,
      ToolErrorCode: '',
      Status: 'running',
      IsFinal: false,
      StartedAt: '',
      CompletedAt: null,
    };
    s.upsertStep(step);
    s.appendStepDelta('s1', 'content', 'A', 10);
    // 重发（同 seq）→ 丢弃。
    s.appendStepDelta('s1', 'content', 'A', 10);
    expect(s.steps.get('s1')?.Content).toBe('A');
    // 乱序旧包（更小 seq）→ 丢弃。
    s.appendStepDelta('s1', 'content', 'B', 9);
    expect(s.steps.get('s1')?.Content).toBe('A');
    // 新包（更大 seq）→ 应用。
    s.appendStepDelta('s1', 'content', 'C', 11);
    expect(s.steps.get('s1')?.Content).toBe('AC');
    // 不同 field 独立记录 lastSeen：reasoning 从更小 seq 开始也应用。
    s.appendStepDelta('s1', 'reasoning', 'R', 3);
    expect(s.steps.get('s1')?.Reasoning).toBe('R');
    // 无 seq 的边界包不受 lastSeen 影响，总是应用。
    s.appendStepDelta('s1', 'content', 'D');
    expect(s.steps.get('s1')?.Content).toBe('ACD');
  });

  it('getSessionTasks returns tasks sorted by seq', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't2', Seq: 2 }));
    s.upsertTask(makeTask({ ID: 't1', Seq: 1 }));
    const tasks = s.getSessionTasks('s1');
    expect(tasks.map((t) => t.ID)).toEqual(['t1', 't2']);
  });

  it('clearSession removes entities by spirit session id', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't1', SessionID: 's1' }));
    s.upsertTask(makeTask({ ID: 't2', SessionID: 's2' }));
    s.clearSession('s1');
    expect(s.tasks.has('t1')).toBe(false);
    expect(s.tasks.has('t2')).toBe(true);
  });

  // P2-07: sub-resource fetch failures must be recorded in hydrationErrors
  // instead of being silently swallowed.
  it('fetchSessionHistory records sub-resource failures in hydrationErrors', async () => {
    vi.mocked(listTasksV2).mockResolvedValue([makeTask({ ID: 'task-1' })]);
    vi.mocked(listStepsV2).mockResolvedValue([]);
    vi.mocked(listTurnsV2).mockRejectedValue(new Error('turns API down'));
    vi.mocked(listTeamStagesV2).mockResolvedValue([]);
    vi.mocked(listPlanBoardsV2).mockResolvedValue([]);
    vi.mocked(listPlanStepsV2).mockResolvedValue([]);
    vi.mocked(listGraphStagesV2).mockResolvedValue([]);
    vi.mocked(listOrphanMemberSessionsV2).mockResolvedValue([]);

    const s = useChatActivityStore();
    await s.fetchSessionHistory('sess-1');
    await flushPromises(); // per-task hydration is fire-and-forget now

    expect(s.hydrationErrors.length).toBe(1);
    expect(s.hydrationErrors[0].scope).toBe('turns');
    expect(s.hydrationErrors[0].parentId).toBe('task-1');
    expect(s.hydrationErrors[0].message).toBe('turns API down');
  });

  it('fetchSessionHistory clears hydrationErrors at start', async () => {
    vi.mocked(listTasksV2).mockResolvedValue([]);
    vi.mocked(listStepsV2).mockResolvedValue([]);
    vi.mocked(listOrphanMemberSessionsV2).mockResolvedValue([]);

    const s = useChatActivityStore();
    // Simulate a previous failed fetch.
    s.hydrationErrors.push({ scope: 'turns', parentId: 'old', message: 'old error' });
    expect(s.hydrationErrors.length).toBe(1);

    await s.fetchSessionHistory('sess-2');
    expect(s.hydrationErrors.length).toBe(0);
  });

  it('fetchSessionHistory hydrates Mode B orphan member sessions', async () => {
    vi.mocked(listTasksV2).mockResolvedValue([makeTask({ ID: 'task-1', SessionID: 'spirit-1', Status: 'running' })]);
    vi.mocked(listStepsV2).mockResolvedValue([]);
    vi.mocked(listTurnsV2).mockResolvedValue([]);
    vi.mocked(listTeamStagesV2).mockResolvedValue([]);
    vi.mocked(listPlanBoardsV2).mockResolvedValue([]);
    vi.mocked(listPlanStepsV2).mockResolvedValue([]);
    vi.mocked(listGraphStagesV2).mockResolvedValue([]);
    vi.mocked(listOrphanMemberSessionsV2).mockResolvedValue([
      {
        ID: 'ms-orphan',
        TeamRunID: '',
        TeamStageID: '',
        TaskID: '',
        SessionID: 'child-sess',
        SpiritSessionID: 'spirit-1',
        AgentKey: 'subagent:r1',
        AgentName: 'Do stuff',
        AvatarURL: '',
        Status: 'running',
        Seq: 1,
        Version: 1,
        StartedAt: '',
        FinishedAt: null,
        Error: '',
      },
    ]);

    const s = useChatActivityStore();
    await s.fetchSessionHistory('spirit-1');

    expect(listOrphanMemberSessionsV2).toHaveBeenCalledWith('spirit-1');
    expect(s.memberSessions.get('ms-orphan')?.SessionID).toBe('child-sess');
    expect(s.getTaskOrphanMemberSessions('task-1').map((m) => m.ID)).toEqual(['ms-orphan']);
  });
  it('appendStepDelta ignores unknown DeltaField without corrupting Reasoning', () => {
    const s = useChatActivityStore();
    const step: Step = {
      ID: 's1',
      TurnID: 't1',
      TaskID: 'tk1',
      SessionID: 's1',
      SpiritSessionID: 's1',
      Kind: 'reply',
      AuthorAgentKey: 'a1',
      Seq: 1,
      Version: 1,
      Content: '',
      Reasoning: 'existing',
      ToolName: '',
      ToolCallID: '',
      ToolArgs: null,
      ToolResult: null,
      ToolDurationMs: 0,
      ToolErrorCode: '',
      Status: 'running',
      IsFinal: false,
      StartedAt: '',
      CompletedAt: null,
    };
    s.upsertStep(step);
    s.appendStepDelta('s1', 'tool_args', '{"arg":1}');
    expect(s.steps.get('s1')?.Content).toBe('');
    expect(s.steps.get('s1')?.Reasoning).toBe('existing');
  });

  it('getTaskOrphanMemberSessions returns Mode B cards with empty TeamRunID/TaskID on host task', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't1', SessionID: 'spirit-1', Status: 'running', Seq: 1 }));
    s.upsertMemberSession({
      ID: 'ms-modeb',
      TeamRunID: '',
      TeamStageID: '',
      TaskID: '',
      SessionID: 'child-sess',
      SpiritSessionID: 'spirit-1',
      AgentKey: 'subagent:run1',
      AgentName: 'Do stuff',
      AvatarURL: '',
      Status: 'running',
      Seq: 1,
      Version: 1,
      StartedAt: '',
      FinishedAt: null,
      Error: '',
    });
    const orphans = s.getTaskOrphanMemberSessions('t1');
    expect(orphans.map((m) => m.ID)).toEqual(['ms-modeb']);
  });

  it('getTaskOrphanMemberSessions excludes members already under a TeamRun', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't1', SessionID: 'spirit-1', Status: 'running' }));
    s.upsertTeamRun({
      ID: 'tr1',
      TeamStageID: 'ts1',
      TaskID: 't1',
      SessionID: 'spirit-1',
      SpiritSessionID: 'spirit-1',
      DagNodeID: '',
      DependsOn: [],
      Status: 'running',
      StartedAt: '',
      CompletedAt: null,
      Seq: 1,
      Version: 1,
      Error: '',
    });
    s.upsertMemberSession({
      ID: 'ms-team',
      TeamRunID: 'tr1',
      TeamStageID: 'ts1',
      TaskID: 't1',
      SessionID: 'member-sess',
      SpiritSessionID: 'spirit-1',
      AgentKey: 'coder',
      AgentName: 'Coder',
      AvatarURL: '',
      Status: 'running',
      Seq: 1,
      Version: 1,
      StartedAt: '',
      FinishedAt: null,
      Error: '',
    });
    expect(s.getTaskOrphanMemberSessions('t1')).toEqual([]);
  });

  it('hydratedTaskIds starts empty and survives upsertTask (no auto-mark on bulk path)', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't1' }));
    // upsertTask 不自动标记——历史任务经 fetchSessionHistory 批量 upsert，不能误判为水合。
    expect(s.hydratedTaskIds.has('t1')).toBe(false);
  });

  it('clearAll resets hydration tracking', () => {
    const s = useChatActivityStore();
    s.hydratedTaskIds.add('t1');
    s.taskHydration.set('t1', 'loading');
    s.clearAll();
    expect(s.hydratedTaskIds.size).toBe(0);
    expect(s.taskHydration.size).toBe(0);
  });

  // ── R4: memory_recalled recall transparency indexing ──────────────────

  const recallPayload = JSON.stringify({
    hits: [
      { layer: 'L3', line: '用户偏好 XX 餐厅', score: 0.91, fact_id: 'f-1' },
      { layer: 'L2', line: '上次聚餐点了日料', score: 0.72 },
    ],
  });

  it('upsertStep indexes memory_recalled hits by turn ID', () => {
    const s = useChatActivityStore();
    s.upsertStep(makeNoticeStep({ Content: recallPayload }));
    const hits = s.getTurnRecallHits('turn-1');
    expect(hits).toHaveLength(2);
    expect(hits[0]).toMatchObject({ layer: 'L3', line: '用户偏好 XX 餐厅', score: 0.91 });
  });

  it('does not index non-memory notices or invalid payloads', () => {
    const s = useChatActivityStore();
    s.upsertStep(makeNoticeStep({ ID: 'n2', NoticeType: 'model_router', Content: 'switched' }));
    s.upsertStep(makeNoticeStep({ ID: 'n3', Content: 'not json' }));
    s.upsertStep(makeNoticeStep({ ID: 'n4', Content: '{"hits":[]}' }));
    expect(s.getTurnRecallHits('turn-1')).toEqual([]);
  });

  it('re-upsert (created → completed) keeps hits idempotent', () => {
    const s = useChatActivityStore();
    s.upsertStep(makeNoticeStep({ Content: recallPayload, Status: 'running', Version: 1 }));
    s.upsertStep(makeNoticeStep({ Content: recallPayload, Status: 'completed', Version: 2 }));
    expect(s.getTurnRecallHits('turn-1')).toHaveLength(2);
  });

  it('clearSession removes recall hits of session turns', () => {
    const s = useChatActivityStore();
    s.upsertTurn({
      ID: 'turn-1',
      TaskID: 'task-1',
      SessionID: 'sess-1',
      SpiritSessionID: 'spirit-1',
      ParentTurnID: '',
      AgentKey: 'a1',
      TeamID: '',
      TeamStageID: '',
      Seq: 1,
      Version: 1,
      Status: 'running',
      StartedAt: '',
      CompletedAt: null,
    });
    s.upsertStep(makeNoticeStep({ Content: recallPayload }));
    expect(s.getTurnRecallHits('turn-1')).toHaveLength(2);
    s.clearSession('spirit-1');
    expect(s.getTurnRecallHits('turn-1')).toEqual([]);
  });

  it('clearAll clears recall hits', () => {
    const s = useChatActivityStore();
    s.upsertStep(makeNoticeStep({ Content: recallPayload }));
    s.clearAll();
    expect(s.getTurnRecallHits('turn-1')).toEqual([]);
  });
});

describe('knowledge_recalled notice indexing', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    clearAllStepLiveText();
  });

  const kbPayload = JSON.stringify({
    chunks: [
      { chunk_id: 'k1', doc_id: 'd1', score: 0.91, line: 'SLA 承诺 99.9%' },
      { chunk_id: 'k2', doc_id: 'd2', score: 0.4, line: '值班电话' },
    ],
  });

  it('indexes knowledge_recalled chunks by turn and merges later notices', () => {
    const s = useChatActivityStore();
    s.upsertStep(makeNoticeStep({ NoticeType: 'knowledge_recalled', Content: kbPayload }));
    expect(s.getTurnKnowledgeChunks('turn-1')).toHaveLength(2);
    s.upsertStep(
      makeNoticeStep({
        ID: 'n-kb-2',
        NoticeType: 'knowledge_recalled',
        Content: JSON.stringify({ chunks: [{ chunk_id: 'k1', line: 'dup' }, { chunk_id: 'k3', line: 'more' }] }),
      }),
    );
    const chunks = s.getTurnKnowledgeChunks('turn-1');
    expect(chunks.map((c) => c.chunk_id)).toEqual(['k1', 'k2', 'k3']);
  });

  it('clearAll clears knowledge chunks', () => {
    const s = useChatActivityStore();
    s.upsertStep(makeNoticeStep({ NoticeType: 'knowledge_recalled', Content: kbPayload }));
    s.clearAll();
    expect(s.getTurnKnowledgeChunks('turn-1')).toEqual([]);
  });
});

describe('fetchSessionHistory phased hydration', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    clearAllStepLiveText();
    vi.clearAllMocks();
    vi.mocked(listOrphanMemberSessionsV2).mockResolvedValue([]);
  });

  it('Phase 1 fetches steps with limit window, not full list', async () => {
    vi.mocked(listTasksV2).mockResolvedValue([]);
    vi.mocked(listStepsV2).mockResolvedValue([]);
    const s = useChatActivityStore();
    await s.fetchSessionHistory('sess-1');
    expect(listStepsV2).toHaveBeenCalledWith('sess-1', { limit: 100 });
  });

  it('auto-hydrates only last + non-terminal tasks; terminal history stays collapsed', async () => {
    vi.mocked(listTasksV2).mockResolvedValue([
      makeTask({ ID: 't-old', SessionID: 'sess-1', Status: 'completed', Seq: 1 }),
      makeTask({ ID: 't-mid', SessionID: 'sess-1', Status: 'completed', Seq: 2 }),
      makeTask({ ID: 't-last', SessionID: 'sess-1', Status: 'completed', Seq: 3 }),
    ]);
    vi.mocked(listStepsV2).mockResolvedValue([]);
    vi.mocked(listTurnsV2).mockResolvedValue([]);
    vi.mocked(listTeamStagesV2).mockResolvedValue([]);
    vi.mocked(listPlanBoardsV2).mockResolvedValue([]);
    vi.mocked(listPlanStepsV2).mockResolvedValue([]);
    vi.mocked(listGraphStagesV2).mockResolvedValue([]);

    const s = useChatActivityStore();
    await s.fetchSessionHistory('sess-1');
    await flushPromises();

    // 只有最后一个 task 被自动水合 → listTurnsV2 只调用 1 次
    expect(listTurnsV2).toHaveBeenCalledTimes(1);
    expect(listTurnsV2).toHaveBeenCalledWith('t-last');
    expect(s.hydratedTaskIds.has('t-last')).toBe(true);
    expect(s.hydratedTaskIds.has('t-old')).toBe(false);
    expect(s.hydratedTaskIds.has('t-mid')).toBe(false);
  });

  it('auto-hydrates non-terminal tasks (running/pending/interrupted) regardless of position', async () => {
    vi.mocked(listTasksV2).mockResolvedValue([
      makeTask({ ID: 't-run', SessionID: 'sess-1', Status: 'running', Seq: 1 }),
      makeTask({ ID: 't-int', SessionID: 'sess-1', Status: 'interrupted', Seq: 2 }),
      makeTask({ ID: 't-done', SessionID: 'sess-1', Status: 'completed', Seq: 3 }),
    ]);
    vi.mocked(listStepsV2).mockResolvedValue([]);
    vi.mocked(listTurnsV2).mockResolvedValue([]);
    vi.mocked(listTeamStagesV2).mockResolvedValue([]);
    vi.mocked(listPlanBoardsV2).mockResolvedValue([]);
    vi.mocked(listPlanStepsV2).mockResolvedValue([]);
    vi.mocked(listGraphStagesV2).mockResolvedValue([]);

    const s = useChatActivityStore();
    await s.fetchSessionHistory('sess-1');
    await flushPromises();

    const hydrated = new Set(vi.mocked(listTurnsV2).mock.calls.map((c) => c[0]));
    expect(hydrated).toEqual(new Set(['t-run', 't-int', 't-done'])); // t-done 是最后一个
  });

  it('hydratedTaskIds survive a re-fetch (WS reconnect keeps cards expanded)', async () => {
    vi.mocked(listTasksV2).mockResolvedValue([makeTask({ ID: 't-1', SessionID: 'sess-1', Status: 'completed' })]);
    vi.mocked(listStepsV2).mockResolvedValue([]);
    vi.mocked(listTurnsV2).mockResolvedValue([]);
    vi.mocked(listTeamStagesV2).mockResolvedValue([]);
    vi.mocked(listPlanBoardsV2).mockResolvedValue([]);
    vi.mocked(listPlanStepsV2).mockResolvedValue([]);
    vi.mocked(listGraphStagesV2).mockResolvedValue([]);

    const s = useChatActivityStore();
    await s.fetchSessionHistory('sess-1');
    await flushPromises();
    expect(s.hydratedTaskIds.has('t-1')).toBe(true);

    // 重连再拉：即便 t-1 已非「最后+非终态」之外的逻辑，仍因已水合而保持
    await s.fetchSessionHistory('sess-1');
    await flushPromises();
    expect(s.hydratedTaskIds.has('t-1')).toBe(true);
  });
});

describe('hydrateTask', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
    vi.mocked(listOrphanMemberSessionsV2).mockResolvedValue([]);
  });

  function seedOneCompletedTask(): ReturnType<typeof useChatActivityStore> {
    vi.mocked(listTasksV2).mockResolvedValue([makeTask({ ID: 't-1', SessionID: 'sess-1', Status: 'completed' })]);
    vi.mocked(listStepsV2).mockResolvedValue([]);
    const s = useChatActivityStore();
    return s;
  }

  it('is idempotent: concurrent calls trigger only one fetch round', async () => {
    const s = seedOneCompletedTask();
    vi.mocked(listTurnsV2).mockResolvedValue([]);
    vi.mocked(listTeamStagesV2).mockResolvedValue([]);
    vi.mocked(listPlanBoardsV2).mockResolvedValue([]);
    vi.mocked(listPlanStepsV2).mockResolvedValue([]);
    vi.mocked(listGraphStagesV2).mockResolvedValue([]);

    // 注意：t-1 是最后一个 task，fetchSessionHistory 已触发一次 hydrate；
    // 这里直接测 store action 本身——先造一个未水合的 task。
    const s2 = useChatActivityStore();
    s2.upsertTask(makeTask({ ID: 't-9', SessionID: 'sess-1', Status: 'completed' }));
    await Promise.all([s2.hydrateTask('t-9'), s2.hydrateTask('t-9')]);
    const calls = vi.mocked(listTurnsV2).mock.calls.filter((c) => c[0] === 't-9');
    expect(calls.length).toBe(1);
    expect(s2.hydratedTaskIds.has('t-9')).toBe(true);
    void s;
  });

  it('sets error state on sub-resource failure and allows retry', async () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't-9', SessionID: 'sess-1', Status: 'completed' }));

    vi.mocked(listTurnsV2).mockRejectedValueOnce(new Error('turns down'));
    vi.mocked(listStepsV2).mockResolvedValue([]);
    vi.mocked(listTeamStagesV2).mockResolvedValue([]);
    vi.mocked(listPlanBoardsV2).mockResolvedValue([]);
    vi.mocked(listPlanStepsV2).mockResolvedValue([]);
    vi.mocked(listGraphStagesV2).mockResolvedValue([]);

    await s.hydrateTask('t-9');
    expect(s.taskHydration.get('t-9')).toBe('error');
    expect(s.hydratedTaskIds.has('t-9')).toBe(false);

    // 重试成功
    vi.mocked(listTurnsV2).mockResolvedValue([]);
    await s.hydrateTask('t-9');
    expect(s.taskHydration.get('t-9')).toBeUndefined();
    expect(s.hydratedTaskIds.has('t-9')).toBe(true);
  });
});
