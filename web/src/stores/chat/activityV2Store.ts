// web/src/stores/chat/activityV2Store.ts
import { ref } from 'vue';
import { defineStore } from 'pinia';
import type {
  Task,
  Turn,
  Step,
  TeamStage,
  TeamRun,
  MemberSession,
  PlanBoard,
  PlanStep,
  GraphStage,
  GraphNode,
} from '../../features/chat/v2Types';
import {
  listTasksV2,
  listTurnsV2,
  listStepsV2,
  listTeamStagesV2,
  listTeamRunsV2,
  listMemberSessionsV2,
  listOrphanMemberSessionsV2,
  listPlanBoardsV2,
  listPlanStepsV2,
  listGraphStagesV2,
  listGraphNodesV2,
} from '../../features/session/v2Api';
import { useNodeOutputStore } from './nodeOutputStore';
import { MEDIA_TOOL_NAMES, type MediaArtifact } from '../../features/chat/mediaTypes';
import {
  noteStepLiveText,
  mergeStepLiveText,
  clearStepLiveText,
  clearAllStepLiveText,
  isTerminalStepStatus,
} from '../../features/chat/stepLiveTextCache';
import {
  MEMORY_RECALLED_NOTICE_TYPE,
  parseMemoryRecallHits,
  type MemoryRecallHit,
} from '../../features/chat/memoryRecall';
import {
  KNOWLEDGE_RECALLED_NOTICE_TYPE,
  parseKnowledgeRecallChunks,
  mergeKnowledgeRecallChunks,
  type KnowledgeRecallChunk,
} from '../../features/chat/knowledgeRecall';

// P2-07: record sub-resource fetch failures during history hydration.
export interface HydrationError {
  scope: string; // entity type: "turns" | "team_stages" | "team_runs" | ...
  parentId: string; // parent entity ID (task ID, team_stage ID, etc.)
  message: string; // error message
}

/**
 * useChatActivityStore holds all v2 chat entities in flat Maps keyed by ID.
 * Entities are keyed by their own ID; associations are via task_id / turn_id etc.
 *
 * Optimistic concurrency: upsert methods reject updates with Version <= existing.
 */
export const useChatActivityStore = defineStore('chatActivityV2', () => {
  const tasks = ref(new Map<string, Task>());
  const turns = ref(new Map<string, Turn>());
  const steps = ref(new Map<string, Step>());
  const teamStages = ref(new Map<string, TeamStage>());
  const teamRuns = ref(new Map<string, TeamRun>());
  const memberSessions = ref(new Map<string, MemberSession>());
  const planBoards = ref(new Map<string, PlanBoard>());
  const planSteps = ref(new Map<string, PlanStep>());
  const graphStages = ref(new Map<string, GraphStage>());
  const graphNodes = ref(new Map<string, GraphNode>());
  // R4 recall transparency: parsed memory_recalled notice hits keyed by turn ID.
  // Populated in upsertStep; the raw notice step stays in `steps` (inspector
  // visibility) but is hidden from the chat stream via noticeFilter.
  const recallHitsByTurn = ref(new Map<string, MemoryRecallHit[]>());
  const knowledgeChunksByTurn = ref(new Map<string, KnowledgeRecallChunk[]>());

  // P2-07: track sub-resource fetch failures so the UI can distinguish
  // "no data" from "failed to load" and show a partial/stale indicator.
  const hydrationErrors = ref<HydrationError[]>([]);
  /** Cache of member session IDs whose steps were lazy-loaded (A.4.7). */
  const loadedMemberStepSessions = ref(new Set<string>());

  // === Lazy hydration state (chat history lazy load, 2026-07-23 design) ===
  // hydratedTaskIds: tasks whose execution subtree has been loaded. Persists
  // across WS reconnects (fetchSessionHistory never clears it) so expanded
  // cards stay expanded; only clearAll/clearSession reset it.
  const hydratedTaskIds = ref(new Set<string>());
  // taskHydration: transient per-task fetch state ('loading' | 'error').
  const taskHydration = ref(new Map<string, 'loading' | 'error'>());

  function catchHydrationError<T>(scope: string, parentId: string): (e: unknown) => T[] {
    return (e: unknown) => {
      hydrationErrors.value.push({
        scope,
        parentId,
        message: e instanceof Error ? e.message : String(e),
      });
      return [] as T[];
    };
  }

  // Phase 1 recent-steps window — covers spirit-level orphan steps and gives
  // the latest task immediate context (design §4.2 Phase 1).
  const HISTORY_STEP_WINDOW = 100;
  // Non-terminal task statuses always auto-hydrate on session open (P5):
  // running/pending 进行态 + interrupted（「继续执行」按钮必须直接可见）。
  const AUTO_HYDRATE_STATUSES = new Set<Task['Status']>(['pending', 'running', 'interrupted']);

  const nodeOutputStore = useNodeOutputStore();

  // === Upsert helpers (optimistic-concurrency guarded) ===

  /** Go 零值 time.Time 序列化为 "0001-01-01T00:00:00Z"，merge 时不能覆盖已有值。 */
  function isZeroTime(s: string): boolean {
    return !s || s.startsWith('0001-01-01');
  }

  function upsertTask(t: Task) {
    const ex = tasks.value.get(t.ID);
    if (ex && t.Version <= ex.Version) return;
    const merged: Task = ex
      ? {
          ...ex,
          ...t,
          UserMessage: t.UserMessage || ex.UserMessage,
          CreatedAt: isZeroTime(t.CreatedAt) ? ex.CreatedAt : t.CreatedAt,
          UpdatedAt: isZeroTime(t.UpdatedAt) ? ex.UpdatedAt : t.UpdatedAt,
          CompletedAt: t.CompletedAt ?? ex.CompletedAt ?? null,
        }
      : { ...t };
    tasks.value.set(t.ID, merged);
    if (!t.ID.startsWith('pending-user-')) {
      for (const [id, task] of tasks.value) {
        if (id.startsWith('pending-user-') && task.SessionID === t.SessionID) {
          tasks.value.delete(id);
        }
      }
    }
  }

  /** removeTask 删除指定 ID 的 Task（用于乐观 Task 失败时清理） */
  function removeTask(taskId: string) {
    tasks.value.delete(taskId);
  }

  function upsertTurn(t: Turn) {
    const ex = turns.value.get(t.ID);
    if (ex && t.Version <= ex.Version) return;
    turns.value.set(t.ID, { ...t });
  }

  function upsertStep(s: Step) {
    const ex = steps.value.get(s.ID);
    if (ex && s.Version < ex.Version) return;
    // P3-resume：终态到达（completed/failed/...）清除 live-text 缓存；
    // 非终态则合并缓存（前缀一致取长者），刷新后 hydrate 的 DB 快照可能被
    // 本 tab 更新的缓存补足。
    if (isTerminalStepStatus(s.Status)) {
      clearStepLiveText(s.ID);
    } else {
      s = mergeStepLiveText(s);
    }
    // For same version (streaming updates), merge content fields instead of replacing
    if (ex && s.Version === ex.Version) {
      steps.value.set(s.ID, { ...ex, ...s });
    } else {
      steps.value.set(s.ID, { ...s });
    }
    // Sync media outputs to nodeOutputStore for observation canvas.
    // When a media tool completes, extract artifacts from ToolResult and map to the node.
    if (s.Kind === 'action' && s.Status === 'completed' && s.ToolName) {
      if (MEDIA_TOOL_NAMES.includes(s.ToolName)) {
        const result = s.ToolResult as Record<string, unknown> | null;
        const artifacts = result?.artifacts;
        if (Array.isArray(artifacts) && artifacts.length > 0) {
          // Step has no TeamStageID field; map agent key to node ID.
          const nodeId = s.AuthorAgentKey;
          if (nodeId) {
            nodeOutputStore.setNodeOutput(nodeId, artifacts as MediaArtifact[]);
          }
        }
      }
    }
    // R4 recall transparency: index memory_recalled notice hits by turn.
    // Idempotent — re-upserts (created → completed) overwrite with the same hits.
    if (s.Kind === 'notice' && s.NoticeType === MEMORY_RECALLED_NOTICE_TYPE && s.TurnID) {
      const hits = parseMemoryRecallHits(s.Content);
      if (hits.length > 0) {
        recallHitsByTurn.value.set(s.TurnID, hits);
      }
    }
    if (s.Kind === 'notice' && s.NoticeType === KNOWLEDGE_RECALLED_NOTICE_TYPE && s.TurnID) {
      const incoming = parseKnowledgeRecallChunks(s.Content);
      if (incoming.length > 0) {
        const prev = knowledgeChunksByTurn.value.get(s.TurnID) ?? [];
        knowledgeChunksByTurn.value.set(s.TurnID, mergeKnowledgeRecallChunks(prev, incoming));
      }
    }
  }

  function upsertTeamStage(ts: TeamStage) {
    const ex = teamStages.value.get(ts.ID);
    if (ex && ts.Version <= ex.Version) return;
    if (ex && (!ts.Members || ts.Members.length === 0) && ex.Members && ex.Members.length > 0) {
      ts = { ...ts, Members: ex.Members };
    }
    teamStages.value.set(ts.ID, { ...ts });
  }

  function upsertTeamRun(tr: TeamRun) {
    const ex = teamRuns.value.get(tr.ID);
    if (ex && tr.Version <= ex.Version) return;
    teamRuns.value.set(tr.ID, { ...tr });
  }

  function upsertMemberSession(ms: MemberSession) {
    const ex = memberSessions.value.get(ms.ID);
    if (ex && ms.Version <= ex.Version) return;
    if (ex) {
      memberSessions.value.set(ms.ID, { ...ex, ...ms });
    } else {
      memberSessions.value.set(ms.ID, { ...ms });
    }
  }

  function upsertPlanBoard(pb: PlanBoard) {
    const ex = planBoards.value.get(pb.ID);
    if (ex && pb.Version <= ex.Version) return;
    planBoards.value.set(pb.ID, { ...pb });
    if (pb.Steps && pb.Steps.length > 0) {
      for (const ps of pb.Steps) {
        const exStep = planSteps.value.get(ps.ID);
        if (!exStep || ps.Version > exStep.Version) {
          planSteps.value.set(ps.ID, { ...ps });
        }
      }
    }
  }

  function upsertPlanStep(ps: PlanStep) {
    const ex = planSteps.value.get(ps.ID);
    if (ex && ps.Version < ex.Version) return;
    if (ex && ps.Version === ex.Version) {
      planSteps.value.set(ps.ID, { ...ex, ...ps });
    } else {
      planSteps.value.set(ps.ID, { ...ps });
    }
  }

  function upsertGraphStage(gs: GraphStage) {
    const ex = graphStages.value.get(gs.ID);
    if (ex && gs.Version <= ex.Version) return;
    graphStages.value.set(gs.ID, { ...gs });
    if (gs.Nodes && gs.Nodes.length > 0) {
      for (const gn of gs.Nodes) {
        // GraphNode 无 Version 字段，直接覆盖
        graphNodes.value.set(gn.ID, { ...gn });
      }
    }
  }

  function upsertGraphNode(gn: GraphNode) {
    const ex = graphNodes.value.get(gn.ID);
    if (!ex) {
      graphNodes.value.set(gn.ID, { ...gn });
      return;
    }
    // 合并：终态事件可能不带 TeamStageID/DependsOn/Label，不得用空值擦除已有字段。
    graphNodes.value.set(gn.ID, {
      ...ex,
      ...gn,
      TeamStageID: gn.TeamStageID || ex.TeamStageID,
      DependsOn: gn.DependsOn?.length ? gn.DependsOn : ex.DependsOn,
      Label: gn.Label || ex.Label,
      DagNodeID: gn.DagNodeID || ex.DagNodeID,
    });
  }

  // === Streaming delta (does NOT bump version) ===

  // P3 fix: dedup redelivered/reordered streaming deltas by the session-scoped
  // monotonic DeltaSeq assigned by the backend Sequencer at flush time.
  // Key = stepId:field; value = last applied DeltaSeq. A delta with
  // DeltaSeq <= lastSeen is a redelivery and is dropped. Deltas without a seq
  // (legacy/edge producers) are always applied — no content fingerprinting,
  // which falsely killed legitimately repeated chunks (e.g. "哈哈哈" arriving
  // as three separate "哈" deltas).
  const lastDeltaSeqs = new Map<string, number>();

  function shouldApplyDelta(stepId: string, field: string, deltaSeq?: number): boolean {
    if (deltaSeq === undefined || deltaSeq === null || deltaSeq <= 0) return true;
    const key = `${stepId}:${field}`;
    const last = lastDeltaSeqs.get(key) ?? 0;
    if (deltaSeq <= last) return false;
    lastDeltaSeqs.set(key, deltaSeq);
    return true;
  }

  // P2-06 fix: accept any DeltaField string from the backend and handle only
  // the known string-appendable fields (content, reasoning). Unknown fields
  // (e.g. tool_args, state) are silently ignored — the final value arrives
  // via the subsequent step.completed event which carries the complete Step
  // entity. The previous if/else caught ALL non-content fields and appended
  // them to Reasoning, causing silent data corruption.
  function appendStepDelta(stepId: string, field: string, chunk: string, deltaSeq?: number) {
    const s = steps.value.get(stepId);
    if (!s) return;
    if (!chunk) return;
    if (!shouldApplyDelta(stepId, field, deltaSeq)) return;
    switch (field) {
      case 'content':
        s.Content += chunk;
        // P3-resume：累积值写 live-text 缓存（节流 flush sessionStorage），
        // 刷新后 hydrate 合并填补文本空洞。
        noteStepLiveText(stepId, 'content', s.Content);
        break;
      case 'reasoning':
        s.Reasoning += chunk;
        noteStepLiveText(stepId, 'reasoning', s.Reasoning);
        break;
      default:
        // Unknown delta field — ignore; final value comes from step.completed.
        break;
    }
  }

  // === Query helpers ===

  /**
   * Primary sort: Seq ASC (creation-time order assigned by backend, immutable);
   * StartedAt/CreatedAt as tiebreaker and as fallback for legacy rows without Seq.
   *
   * 2026-08-06 顺序修复（20:45 会话看板乱序根因）：之前以 StartedAt 主排序，
   * 但 PlanStep.StartedAt 会在 dispatch 时被覆盖为实际开始时间，导致已运行
   * step 排到 pending step（保留创建时间）之后，看板随执行进度乱序。Seq 在
   * 创建时确定且不可变，作为主键稳定。任一方 Seq 缺失（=0，遗留数据）时
   * 回退到 time 排序，保持旧行为。
   */
  function compareBySeqThenTime(
    a: { Seq: number; StartedAt?: string; CreatedAt?: string },
    b: { Seq: number; StartedAt?: string; CreatedAt?: string },
  ): number {
    const sa = a.Seq || 0;
    const sb = b.Seq || 0;
    if (sa > 0 && sb > 0 && sa !== sb) return sa - sb;
    const ta = Date.parse(a.StartedAt || a.CreatedAt || '') || 0;
    const tb = Date.parse(b.StartedAt || b.CreatedAt || '') || 0;
    if (ta !== tb) return ta - tb;
    return sa - sb;
  }

  function getSessionTasks(sessionId: string): Task[] {
    const out: Task[] = [];
    for (const t of tasks.value.values()) {
      if (t.SessionID === sessionId) out.push(t);
    }
    return out.sort(compareBySeqThenTime);
  }

  function getTaskTurns(taskId: string): Turn[] {
    const out: Turn[] = [];
    for (const t of turns.value.values()) {
      if (t.TaskID === taskId) out.push(t);
    }
    return out.sort(compareBySeqThenTime);
  }

  function getTurnSteps(turnId: string): Step[] {
    const out: Step[] = [];
    for (const s of steps.value.values()) {
      if (s.TurnID === turnId) out.push(s);
    }
    return out.sort(compareBySeqThenTime);
  }

  /** R4: recall hits injected for a turn (empty array when none). */
  function getTurnRecallHits(turnId: string): MemoryRecallHit[] {
    return recallHitsByTurn.value.get(turnId) ?? [];
  }

  function getTurnKnowledgeChunks(turnId: string): KnowledgeRecallChunk[] {
    return knowledgeChunksByTurn.value.get(turnId) ?? [];
  }

  /** Orphan steps owned directly by a task (TurnID empty), e.g. clarification
   *  gate steps published before the run/turn exists (design: B.10.18). */
  function getTaskOrphanSteps(taskId: string): Step[] {
    const out: Step[] = [];
    for (const s of steps.value.values()) {
      if (s.TaskID === taskId && !s.TurnID) out.push(s);
    }
    return out.sort(compareBySeqThenTime);
  }

  /** 任务最终回复文本：按序取最后一个非空 reply step 的 Content（转评估用例/复制用）。 */
  function getTaskFinalReply(taskId: string): string {
    let last = '';
    const collect = (list: Step[]) => {
      for (const s of list) {
        if (s.Kind === 'reply' && s.Content?.trim()) last = s.Content.trim();
      }
    };
    for (const turn of getTaskTurns(taskId)) collect(getTurnSteps(turn.ID));
    collect(getTaskOrphanSteps(taskId));
    return last;
  }

  function getTaskTeamStages(taskId: string): TeamStage[] {
    const out: TeamStage[] = [];
    for (const ts of teamStages.value.values()) {
      if (ts.TaskID === taskId) out.push(ts);
    }
    return out.sort(compareBySeqThenTime);
  }

  // 按触发 turn 查询 team stages（设计稿 §3.6.2：teamStages.filter(ts => ts.turnId === turn.id)）
  function getTurnTeamStages(turnId: string): TeamStage[] {
    const out: TeamStage[] = [];
    for (const ts of teamStages.value.values()) {
      if (ts.TurnID === turnId) out.push(ts);
    }
    return out.sort(compareBySeqThenTime);
  }

  function getTaskPlanBoards(taskId: string): PlanBoard[] {
    const out: PlanBoard[] = [];
    for (const pb of planBoards.value.values()) {
      if (pb.TaskID === taskId) out.push(pb);
    }
    return out.sort(compareBySeqThenTime);
  }

  function getTaskGraphStages(taskId: string): GraphStage[] {
    const out: GraphStage[] = [];
    for (const gs of graphStages.value.values()) {
      if (gs.TaskID === taskId) out.push(gs);
    }
    return out.sort(compareBySeqThenTime);
  }

  function getGraphStageByPlanBoard(planBoardId: string): GraphStage | undefined {
    for (const gs of graphStages.value.values()) {
      if (gs.PlanBoardID === planBoardId) return gs;
    }
    return undefined;
  }

  function getPlanBoardSteps(planBoardId: string): PlanStep[] {
    const out: PlanStep[] = [];
    for (const ps of planSteps.value.values()) {
      if (ps.PlanID === planBoardId) out.push(ps);
    }
    return out.sort(compareBySeqThenTime);
  }

  // Match steps for a MemberSession.
  // 1) Prefer Step.SessionID === MemberSession.SessionID (lazy-load / Mode B safe)
  // 2) Else Turn.TeamStageID + AuthorAgentKey
  // 3) Fallback AuthorAgentKey + TaskID (legacy; may cross-team)
  function getMemberSessionSteps(memberSession: MemberSession): Step[] {
    const agentKey = memberSession.AgentKey;
    const taskId = memberSession.TaskID;
    const sessionId = memberSession.SessionID;
    // F3 (12:33 修复): guard 仅要求 agentKey。路径 1 按 SessionID 精确匹配，不需要
    // taskId；系统 Agent 成员的 MemberSession 可能无 TaskID，旧 guard 直接返回空
    // 导致成员执行活动面板无内容。
    if (!agentKey) return [];

    const out: Step[] = [];
    if (sessionId) {
      for (const s of steps.value.values()) {
        if (s.SessionID === sessionId) out.push(s);
      }
      if (out.length > 0) return out.sort(compareBySeqThenTime);
    }

    const turnIds = new Set<string>();
    for (const t of turns.value.values()) {
      if (t.TeamStageID && t.TeamStageID === memberSession.TeamStageID && t.AgentKey === agentKey) {
        turnIds.add(t.ID);
      }
    }
    if (turnIds.size > 0) {
      for (const s of steps.value.values()) {
        if (turnIds.has(s.TurnID) && s.AuthorAgentKey === agentKey) {
          out.push(s);
        }
      }
    }

    if (out.length === 0) {
      for (const s of steps.value.values()) {
        if (s.AuthorAgentKey === agentKey && s.TaskID === taskId) {
          out.push(s);
        }
      }
    }

    return out.sort(compareBySeqThenTime);
  }

  /** Mode B / orphan: member sessions on a task not rendered under any TeamRun.
   *  Also hosts Mode B cards with empty TaskID on the session's running (else latest) task. */
  function getTaskOrphanMemberSessions(taskId: string): MemberSession[] {
    const task = tasks.value.get(taskId);
    if (!task) return [];

    const shown = new Set<string>();
    for (const tr of teamRuns.value.values()) {
      if (tr.TaskID !== taskId) continue;
      for (const ms of memberSessions.value.values()) {
        if (ms.TeamRunID === tr.ID) shown.add(ms.ID);
      }
    }

    const sessionTasks = getSessionTasks(task.SessionID);
    const host = sessionTasks.find((t) => t.Status === 'running') ?? sessionTasks[sessionTasks.length - 1] ?? null;
    const isHost = host?.ID === taskId;

    const out: MemberSession[] = [];
    for (const ms of memberSessions.value.values()) {
      if (shown.has(ms.ID)) continue;
      if (ms.TaskID === taskId) {
        out.push(ms);
        continue;
      }
      // Mode B: empty TaskID + matching spirit session → host on latest/running task
      if (isHost && !ms.TaskID && !ms.TeamRunID && ms.SpiritSessionID === task.SessionID) {
        out.push(ms);
      }
    }
    return out.sort(compareBySeqThenTime);
  }

  function getGraphStageNodes(graphStageId: string): GraphNode[] {
    const out: GraphNode[] = [];
    for (const gn of graphNodes.value.values()) {
      if (gn.GraphStageID === graphStageId) out.push(gn);
    }
    // 2026-08-06 顺序修复：GraphNode 无 Seq 字段，Map 迭代顺序 = 事件到达
    // 顺序（懒加载/重放时乱序）。经 DagNodeID（= plan_step.id）派生
    // PlanStep.Seq 排序；无匹配 PlanStep 的节点排最后，JS sort 稳定保持
    // 插入相对顺序。
    const seqOf = (gn: GraphNode): number => {
      const ps = gn.DagNodeID ? planSteps.value.get(gn.DagNodeID) : undefined;
      return ps && ps.Seq > 0 ? ps.Seq : Number.MAX_SAFE_INTEGER;
    };
    return out.sort((a, b) => seqOf(a) - seqOf(b));
  }

  function getTeamStageTeamRuns(teamStageId: string): TeamRun[] {
    const out: TeamRun[] = [];
    for (const tr of teamRuns.value.values()) {
      if (tr.TeamStageID === teamStageId) out.push(tr);
    }
    return out.sort(compareBySeqThenTime);
  }

  function getTeamRunMemberSessions(teamRunId: string): MemberSession[] {
    const out: MemberSession[] = [];
    for (const ms of memberSessions.value.values()) {
      if (ms.TeamRunID === teamRunId) out.push(ms);
    }
    return out.sort(compareBySeqThenTime);
  }

  // === Bulk operations ===

  function clearSession(spiritSessionId: string) {
    for (const [id, t] of tasks.value) {
      if (t.SessionID === spiritSessionId) {
        tasks.value.delete(id);
        hydratedTaskIds.value.delete(id);
        taskHydration.value.delete(id);
      }
    }
    for (const [id, t] of turns.value) {
      if (t.SpiritSessionID === spiritSessionId) {
        turns.value.delete(id);
        recallHitsByTurn.value.delete(id);
        knowledgeChunksByTurn.value.delete(id);
      }
    }
    for (const [id, s] of steps.value) {
      if (s.SpiritSessionID === spiritSessionId) {
        steps.value.delete(id);
        lastDeltaSeqs.delete(`${id}:content`);
        lastDeltaSeqs.delete(`${id}:reasoning`);
        clearStepLiveText(id);
      }
    }
    for (const [id, ts] of teamStages.value) {
      if (ts.SessionID === spiritSessionId) teamStages.value.delete(id);
    }
    for (const [id, tr] of teamRuns.value) {
      if (tr.SpiritSessionID === spiritSessionId) teamRuns.value.delete(id);
    }
    for (const [id, ms] of memberSessions.value) {
      if (ms.SpiritSessionID === spiritSessionId) memberSessions.value.delete(id);
    }
    for (const [id, pb] of planBoards.value) {
      if (pb.SessionID === spiritSessionId) planBoards.value.delete(id);
    }
    for (const [id, ps] of planSteps.value) {
      if (ps.TaskID && tasks.value.has(ps.TaskID) === false) planSteps.value.delete(id);
    }
    for (const [id, gs] of graphStages.value) {
      if (gs.SessionID === spiritSessionId) graphStages.value.delete(id);
    }
    for (const [id, gn] of graphNodes.value) {
      if (gn.GraphStageID && graphStages.value.has(gn.GraphStageID) === false) graphNodes.value.delete(id);
    }
  }

  function clearAll() {
    tasks.value.clear();
    turns.value.clear();
    steps.value.clear();
    lastDeltaSeqs.clear();
    clearAllStepLiveText();
    teamStages.value.clear();
    teamRuns.value.clear();
    memberSessions.value.clear();
    planBoards.value.clear();
    planSteps.value.clear();
    graphStages.value.clear();
    graphNodes.value.clear();
    recallHitsByTurn.value.clear();
    knowledgeChunksByTurn.value.clear();
    loadedMemberStepSessions.value.clear();
    hydratedTaskIds.value.clear();
    taskHydration.value.clear();
  }

  // === History fetch (page refresh / WS reconnect) ===

  /**
   * fetchSessionHistory loads the v2 entity tree in phases (chat history lazy
   * load, 2026-07-23 design §4.2):
   *   Phase 1: tasks (lightweight, all) + recent steps window (limit=100)
   *   Phase 2: compute auto-hydrate set = last task + non-terminal tasks
   *            + already-hydrated tasks (WS reconnect keeps them expanded)
   *   Phase 3: hydrateTask per auto-hydrate task (parallel, fire-and-forget)
   *   Mode B: orphan member_sessions by spirit session (session-level)
   * Historical terminal tasks render as collapsed meta-bar cards and hydrate
   * on demand via hydrateTask (viewport dwell / click).
   */
  async function fetchSessionHistory(sessionId: string): Promise<void> {
    // P2-07: clear previous hydration errors at the start of each fetch.
    hydrationErrors.value = [];

    const [tasksList, stepsList] = await Promise.all([
      listTasksV2(sessionId),
      listStepsV2(sessionId, { limit: HISTORY_STEP_WINDOW }),
    ]);
    for (const t of tasksList) upsertTask(t);
    for (const s of stepsList) upsertStep(s);

    const sorted = [...tasksList].sort(compareBySeqThenTime);
    const autoHydrate = new Set<string>();
    const lastTask = sorted[sorted.length - 1];
    if (lastTask) autoHydrate.add(lastTask.ID);
    for (const t of sorted) {
      if (AUTO_HYDRATE_STATUSES.has(t.Status)) autoHydrate.add(t.ID);
      if (hydratedTaskIds.value.has(t.ID)) autoHydrate.add(t.ID);
    }
    // Fire-and-forget: 首屏不等待执行过程水合，折叠卡即时渲染。
    for (const id of autoHydrate) {
      void hydrateTask(id);
    }

    // Mode B: orphan member sessions (empty TeamRunID) for this spirit session.
    const orphans = await listOrphanMemberSessionsV2(sessionId).catch(
      catchHydrationError<MemberSession>('orphan_member_sessions', sessionId),
    );
    for (const ms of orphans) upsertMemberSession(ms);
  }

  /**
   * hydrateTask loads one task's full execution subtree (turns + task steps +
   * team/plan/graph entities + drill-down runs/sessions/nodes). Idempotent:
   * returns immediately when already hydrated or in flight. On any
   * sub-resource failure the task enters 'error' state (meta-bar retry) and
   * is NOT marked hydrated, so a later expand retries the full fetch.
   */
  async function hydrateTask(taskId: string): Promise<void> {
    const task = tasks.value.get(taskId);
    if (!task) return;
    if (hydratedTaskIds.value.has(taskId)) return;
    if (taskHydration.value.get(taskId) === 'loading') return;
    taskHydration.value.set(taskId, 'loading');

    const errorsBefore = hydrationErrors.value.length;
    const [turnsL, stepsL, teamStagesL, planBoardsL, planStepsL, graphStagesL] = await Promise.all([
      listTurnsV2(taskId).catch(catchHydrationError<Turn>('turns', taskId)),
      listStepsV2(task.SessionID, { taskId }).catch(catchHydrationError<Step>('steps', taskId)),
      listTeamStagesV2(taskId).catch(catchHydrationError<TeamStage>('team_stages', taskId)),
      listPlanBoardsV2(taskId).catch(catchHydrationError<PlanBoard>('plan_boards', taskId)),
      listPlanStepsV2(taskId).catch(catchHydrationError<PlanStep>('plan_steps', taskId)),
      listGraphStagesV2(taskId).catch(catchHydrationError<GraphStage>('graph_stages', taskId)),
    ]);
    for (const turn of turnsL) upsertTurn(turn);
    for (const st of stepsL) upsertStep(st);
    for (const ts of teamStagesL) upsertTeamStage(ts);
    for (const pb of planBoardsL) upsertPlanBoard(pb);
    for (const ps of planStepsL) upsertPlanStep(ps);
    for (const gs of graphStagesL) upsertGraphStage(gs);

    // Drill-down: team_runs → member_sessions (metadata only; member step
    // content stays lazy per A.4.7), graph_nodes per graph_stage.
    const teamRunLists = await Promise.all(
      teamStagesL.map((ts) => listTeamRunsV2(ts.ID).catch(catchHydrationError<TeamRun>('team_runs', ts.ID))),
    );
    const allTeamRuns: TeamRun[] = [];
    for (const runs of teamRunLists) {
      for (const tr of runs) upsertTeamRun(tr);
      allTeamRuns.push(...runs);
    }
    const memberSessionLists = await Promise.all(
      allTeamRuns.map((tr) =>
        listMemberSessionsV2(tr.ID).catch(catchHydrationError<MemberSession>('member_sessions', tr.ID)),
      ),
    );
    for (const sessions of memberSessionLists) {
      for (const ms of sessions) upsertMemberSession(ms);
    }
    const graphNodeLists = await Promise.all(
      graphStagesL.map((gs) => listGraphNodesV2(gs.ID).catch(catchHydrationError<GraphNode>('graph_nodes', gs.ID))),
    );
    for (const nodes of graphNodeLists) {
      for (const gn of nodes) upsertGraphNode(gn);
    }

    if (hydrationErrors.value.length > errorsBefore) {
      taskHydration.value.set(taskId, 'error');
    } else {
      hydratedTaskIds.value.add(taskId);
      taskHydration.value.delete(taskId);
    }
  }

  /** Lazy-load steps for member/child sessions (A.4.7). Cache-aware. */
  async function ensureMemberStepsLoaded(sessionIds: string[]): Promise<void> {
    const pending = sessionIds.filter((id) => id && !loadedMemberStepSessions.value.has(id));
    if (pending.length === 0) return;
    await Promise.all(
      pending.map(async (sid) => {
        try {
          // steps + tasks 并行加载（2026-07-27：tasks 提供成员任务指令 UserMessage，
          // 供 MemberSessionPanel「任务指令」区块展示成员收到的输入内容）
          const [stepsList, tasksList] = await Promise.all([listStepsV2(sid), listTasksV2(sid)]);
          for (const t of tasksList) upsertTask(t);
          for (const s of stepsList) upsertStep(s);
          loadedMemberStepSessions.value.add(sid);
        } catch (e) {
          hydrationErrors.value.push({
            scope: 'member_steps',
            parentId: sid,
            message: e instanceof Error ? e.message : String(e),
          });
        }
      }),
    );
  }

  return {
    tasks,
    turns,
    steps,
    teamStages,
    teamRuns,
    memberSessions,
    planBoards,
    planSteps,
    graphStages,
    graphNodes,
    upsertTask,
    removeTask,
    upsertTurn,
    upsertStep,
    upsertTeamStage,
    upsertTeamRun,
    upsertMemberSession,
    upsertPlanBoard,
    upsertPlanStep,
    upsertGraphStage,
    upsertGraphNode,
    appendStepDelta,
    getSessionTasks,
    getTaskTurns,
    getTurnSteps,
    getTurnRecallHits,
    getTurnKnowledgeChunks,
    getTaskOrphanSteps,
    getTaskFinalReply,
    getTaskTeamStages,
    getTurnTeamStages,
    getTaskPlanBoards,
    getTaskGraphStages,
    getGraphStageByPlanBoard,
    getPlanBoardSteps,
    getMemberSessionSteps,
    getTaskOrphanMemberSessions,
    getGraphStageNodes,
    getTeamStageTeamRuns,
    getTeamRunMemberSessions,
    clearSession,
    clearAll,
    fetchSessionHistory,
    hydrateTask,
    ensureMemberStepsLoaded,
    hydrationErrors,
    hydratedTaskIds,
    taskHydration,
  };
});
