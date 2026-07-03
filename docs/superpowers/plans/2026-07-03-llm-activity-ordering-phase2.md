# LLM Activity Ordering Phase 2: Frontend Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite the frontend chat UI to consume v2 events from the new backend pipeline, replacing the legacy ActivityEvent-based architecture with a flat-Map store + seq-ordered tree rendering.

**Architecture:** A new Pinia store (`useChatActivityStore`) holds flat `Map<string, Entity>` for all 8 v2 entity types. A WS event router dispatches v2 events into store mutations. A tree builder composes `TaskTree` by filtering+sorting entities by `task_id` + `seq`. Vue components render the tree with zero inference (no re-parenting, no dedup, no orphan handling). The legacy compat adapter (`compat_adapter.go`) is removed.

**Tech Stack:** Vue 3 Composition API, Pinia (setup form), TypeScript, Vitest, Quasar

**Spec:** `docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md` §3.6, §9.3, §9.4

---

## File Structure

### New Files (Frontend)

| File | Responsibility |
|------|---------------|
| `web/src/features/chat/v2Types.ts` | TypeScript types for all v2 entities + events + WS envelope |
| `web/src/stores/chat/activityV2Store.ts` | Pinia store: flat Maps for 8 entity types |
| `web/src/features/chat/composables/useChatEventRouter.ts` | Dispatch v2 WS events → store mutations |
| `web/src/features/chat/composables/useTaskTree.ts` | Build TaskTree from store by task_id + seq |
| `web/src/features/chat/composables/usePlanDAGLayout.ts` | SVG DAG layout for plan steps |
| `web/src/components/chat/v2/SessionPanel.vue` | Top-level panel (tasks for current session) |
| `web/src/components/chat/v2/TaskList.vue` | Render tasks sorted by seq |
| `web/src/components/chat/v2/TaskCard.vue` | Single task: user message + status |
| `web/src/components/chat/v2/TurnList.vue` | Render turns sorted by seq |
| `web/src/components/chat/v2/TurnContainer.vue` | Single turn: steps + optional team/plan |
| `web/src/components/chat/v2/PlanBoardCard.vue` | Plan board container |
| `web/src/components/chat/v2/PlanDAG.vue` | SVG dependency graph |
| `web/src/components/chat/v2/PlanStepNode.vue` | DAG node |
| `web/src/components/chat/v2/PlanStepDetailPanel.vue` | Click-to-expand step detail |
| `web/src/components/chat/v2/TeamStagePanel.vue` | Team execution panel |
| `web/src/components/chat/v2/TeamRunCard.vue` | Single team run |
| `web/src/components/chat/v2/MemberSessionPanel.vue` | Member agent panel |

### Modified Files (Frontend)

| File | Change |
|------|-------|
| `web/src/realtime/ws-transport.ts` | Add v2_event dispatch before direction check |
| `web/src/realtime/globalWsHub.ts` | Add onV2Event callback forwarding |
| `web/src/features/chat/composables/useChatWorkspace.ts` | Replace useActivityTimeline with v2 store + router |
| `web/src/components/chat/ThinkingBlock.vue` | Change props from ThinkingEvent to Step |
| `web/src/components/chat/ActionBlock.vue` | Change props from ActionEvent to Step |
| `web/src/components/chat/ReplyBlock.vue` | Change props from ReplyEvent to Step |
| `web/src/features/chat/composables/useBlockedStatus.ts` | Rewrite for v2 tree structure |
| `web/src/features/chat/composables/useChatMessageScroll.ts` | Adapt to TaskTree[] |
| `web/src/features/chat/composables/useScrollToActivity.ts` | Adapt data attributes |
| `web/src/pages/ChatPage.vue` | Wire v2 store + components |
| `web/src/components/chat/ChatMessagePanel.vue` | Replace ActivityStream with v2 SessionPanel |
| `web/src/components/chat/ChatMessageList.vue` | Pass TaskTree[] instead of ActivityTreeNode[] |

### Deleted Files (Frontend — 8 files)

| File | Lines |
|------|-------|
| `web/src/features/chat/composables/useActivityTimeline.ts` | 1410 |
| `web/src/features/chat/activityTypes.ts` | 113 |
| `web/src/features/chat/streamEventTypes.ts` | 256 |
| `web/src/realtime/activityEvent.ts` | 291 |
| `web/src/components/chat/ActivityStream.vue` | 697 |
| `web/src/components/chat/PlanBlock.vue` | 320 |
| `web/src/components/chat/TeamCard.vue` | ~400 |
| `web/src/components/chat/AgentCard.vue` | ~400 |

### Deleted Files (Backend — 1 file)

| File | Reason |
|------|--------|
| `internal/agent/v2/compat_adapter.go` | Phase 2 complete; v1 frontend no longer exists |

### Modified Files (Backend)

| File | Change |
|------|-------|
| `cmd/admin/wire.go` | Remove CompatAdapter provider + binding (if present) |
| `internal/agent/v2/sequencer.go` | Remove CompatAdapter.PublishV1 call (if present) |

---

## Tier 0: Foundation — Types, Store, Event Router, WS Transport

### Task 1: v2 TypeScript Types

**Files:**
- Create: `web/src/features/chat/v2Types.ts`
- Test: `web/src/features/chat/__tests__/v2Types.spec.ts`

- [ ] **Step 1: Write type-validation test**

```typescript
// web/src/features/chat/__tests__/v2Types.spec.ts
import { describe, it, expectTypeOf } from 'vitest';
import type {
  V2WsEnvelope, Task, Turn, Step, TeamStage, TeamRun, MemberSession,
  PlanBoard, PlanStep, StepKind, StepStatus, TaskStatus, TurnStatus,
  TeamStageStatus, TeamRunStatus, MemberSessionStatus, PlanStepStatus,
  EventKind, V2Event,
} from '../v2Types';

describe('v2Types', () => {
  it('V2WsEnvelope has correct shape', () => {
    const env: V2WsEnvelope = { type: 'v2_event', kind: 'task.created', payload: {} as V2Event };
    expectTypeOf(env.type).toEqualTypeOf<string>();
    expectTypeOf(env.kind).toEqualTypeOf<string>();
  });

  it('Task has PascalCase fields matching backend', () => {
    const t: Task = {
      ID: 't1', SessionID: 's1', UserMessage: 'hi', Status: 'running',
      Seq: 1, Version: 1, CreatedAt: '', UpdatedAt: '', CompletedAt: null,
    };
    expectTypeOf(t.ID).toEqualTypeOf<string>();
    expectTypeOf(t.CompletedAt).toEqualTypeOf<string | null>();
  });

  it('Step has PascalCase fields', () => {
    const s: Step = {
      ID: 's1', TurnID: 't1', TaskID: 'tk1', SessionID: 's1',
      SpiritSessionID: 's1', Kind: 'thinking', AuthorAgentKey: 'a1',
      Seq: 1, Version: 1, Content: '', Reasoning: '',
      ToolName: '', ToolCallID: '', ToolArgs: null, ToolResult: null,
      ToolDurationMs: 0, ToolErrorCode: '', Status: 'pending',
      IsFinal: false, StartedAt: '', CompletedAt: null,
    };
    expectTypeOf(s.Kind).toEqualTypeOf<StepKind>();
    expectTypeOf(s.ToolArgs).toEqualTypeOf<unknown | null>();
  });

  it('EventKind constants are string literals', () => {
    const k: EventKind = 'task.created';
    expectTypeOf(k).toEqualTypeOf<EventKind>();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/features/chat/__tests__/v2Types.spec.ts`
Expected: FAIL — cannot find module `../v2Types`

- [ ] **Step 3: Write v2Types.ts**

```typescript
// web/src/features/chat/v2Types.ts

// === Status / Kind string-literal unions ===

export type TaskStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
export type TurnStatus = 'running' | 'completed' | 'failed';
export type StepKind = 'thinking' | 'action' | 'reply' | 'notice' | 'confirm' | 'error';
export type StepStatus =
  | 'pending' | 'running' | 'tool_running' | 'tool_blocked'
  | 'completed' | 'failed' | 'cancelled';
export type TeamStageStatus =
  | 'pending' | 'running' | 'completed' | 'failed' | 'cancelled' | 'waiting_human';
export type TeamStageStage = 'assembled' | 'planning' | 'executing' | 'completed' | 'failed';
export type TeamRunStatus = 'running' | 'completed' | 'failed' | 'cancelled';
export type MemberSessionStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped';
export type PlanStrategy = 'sequential' | 'parallel' | 'dag' | 'coordinator';
export type PlanStatus = 'planning' | 'executing' | 'completed' | 'failed' | 'partial_failure';
export type PlanStepStatus =
  | 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | 'partial_failure';

// === Entity structs (PascalCase JSON keys — no json tags on backend) ===

export interface Task {
  ID: string;
  SessionID: string;
  UserMessage: string;
  Status: TaskStatus;
  Seq: number;
  Version: number;
  CreatedAt: string;
  UpdatedAt: string;
  CompletedAt: string | null;
}

export interface Turn {
  ID: string;
  TaskID: string;
  SessionID: string;
  SpiritSessionID: string;
  ParentTurnID: string;
  AgentKey: string;
  TeamID: string;
  TeamStageID: string;
  Seq: number;
  Version: number;
  Status: TurnStatus;
  StartedAt: string;
  CompletedAt: string | null;
}

export interface Step {
  ID: string;
  TurnID: string;
  TaskID: string;
  SessionID: string;
  SpiritSessionID: string;
  Kind: StepKind;
  AuthorAgentKey: string;
  Seq: number;
  Version: number;
  Content: string;
  Reasoning: string;
  ToolName: string;
  ToolCallID: string;
  ToolArgs: unknown | null;   // json.RawMessage → nested JSON value
  ToolResult: unknown | null;
  ToolDurationMs: number;
  ToolErrorCode: string;
  Status: StepStatus;
  IsFinal: boolean;
  StartedAt: string;
  CompletedAt: string | null;
}

export interface MemberInfo {
  AgentKey: string;
  AgentName: string;
  AvatarURL: string;
  ChildSessionID: string;
  Status: string;
}

export interface TeamStage {
  ID: string;
  TaskID: string;
  TurnID: string;
  SessionID: string;
  TeamID: string;
  DagNodeID: string;
  DependsOn: string[];
  Status: TeamStageStatus;
  Stage: TeamStageStage;
  Members: MemberInfo[];
  Strategy: string;
  StartedAt: string;
  CompletedAt: string | null;
  Seq: number;
  Version: number;
}

export interface TeamRun {
  ID: string;
  TeamStageID: string;
  TaskID: string;
  SessionID: string;
  SpiritSessionID: string;
  DagNodeID: string;
  DependsOn: string[];
  Status: TeamRunStatus;
  StartedAt: string;
  CompletedAt: string | null;
  Seq: number;
  Version: number;
  Error: string;
}

export interface MemberSession {
  ID: string;
  TeamRunID: string;
  TeamStageID: string;
  TaskID: string;
  SessionID: string;
  SpiritSessionID: string;
  AgentKey: string;
  AgentName: string;
  AvatarURL: string;
  Status: MemberSessionStatus;
  Seq: number;
  Version: number;
  StartedAt: string;
  FinishedAt: string | null;
  Error: string;
}

export interface TokenUsage {
  PromptTokens: number;
  CompletionTokens: number;
  TotalTokens: number;
}

export interface MemberReport {
  AgentKey: string;
  AgentName: string;
  Output: string;
  TokensUsed: TokenUsage;
  DurationMs: number;
  Error: string;
}

export interface StepResult {
  Output: string;
  MemberReports: MemberReport[];
  TokensUsed: TokenUsage;
  DurationMs: number;
}

export interface StepError {
  Code: string;
  Message: string;
  Retryable: boolean;
  FailedMember: MemberReport | null;
}

export interface PlanStep {
  ID: string;
  PlanID: string;
  TaskID: string;
  Label: string;
  Description: string;
  DependsOn: string[];
  MappedTeamStageID: string;
  Status: PlanStepStatus;
  AutoSynthesis: boolean;
  StartedAt: string;
  CompletedAt: string | null;
  Seq: number;
  Version: number;
  Result: StepResult | null;
  Error: StepError | null;
}

export interface PlanBoard {
  ID: string;
  TaskID: string;
  TurnID: string;
  SessionID: string;
  Strategy: PlanStrategy;
  Status: PlanStatus;
  Steps: PlanStep[];
  StartedAt: string;
  CompletedAt: string | null;
  Seq: number;
  Version: number;
}

// === EventKind string literals ===

export type EventKind =
  | 'task.created' | 'task.updated' | 'task.completed' | 'task.failed'
  | 'turn.started' | 'turn.completed' | 'turn.failed'
  | 'step.created' | 'step.streaming' | 'step.updated' | 'step.completed' | 'step.failed'
  | 'team_stage.created' | 'team_stage.updated' | 'team_stage.completed' | 'team_stage.failed'
  | 'team_run.started' | 'team_run.completed' | 'team_run.failed'
  | 'member_session.created' | 'member_session.updated'
  | 'plan_board.created' | 'plan_board.updated'
  | 'plan_step.started' | 'plan_step.completed' | 'plan_step.failed'
  | 'plan_step.skipped' | 'plan_step.updated';

// === Event payload shapes (what's inside envelope.payload) ===
// Note: backend event structs have NO json tags, so exported fields
// (PascalCase) are the JSON keys. Unexported fields (taskID, spiritSessionID,
// occurredAt) are NOT serialized.

export interface TaskEventPayload { Task: Task }
export interface TurnEventPayload { TurnID: string; Turn: Turn }
export interface StepCreatedPayload { Step: Step }
export interface StepStreamingPayload { StepID: string; DeltaField: string; DeltaChunk: string }
export interface StepUpdatedPayload { Step: Step }
export interface TeamStageEventPayload { TeamStage: TeamStage }
export interface TeamRunEventPayload { TeamRun: TeamRun }
export interface MemberSessionEventPayload { MemberSession: MemberSession }
export interface PlanBoardEventPayload { PlanBoard: PlanBoard }
export interface PlanStepEventPayload { PlanStep: PlanStep }
export interface PlanStepSkippedPayload { PlanStep: PlanStep; Reason: string }

// Discriminated union of all v2 events
export type V2Event =
  | TaskEventPayload | TurnEventPayload
  | StepCreatedPayload | StepStreamingPayload | StepUpdatedPayload
  | TeamStageEventPayload | TeamRunEventPayload | MemberSessionEventPayload
  | PlanBoardEventPayload | PlanStepEventPayload | PlanStepSkippedPayload;

// === WS envelope ===

export interface V2WsEnvelope {
  type: 'v2_event';
  kind: EventKind;
  payload: V2Event;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/features/chat/__tests__/v2Types.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd web && git add src/features/chat/v2Types.ts src/features/chat/__tests__/v2Types.spec.ts
git commit -m "feat(chat/v2): add TypeScript types for v2 entities and events"
```

---

### Task 2: Pinia Store — activityV2Store

**Files:**
- Create: `web/src/stores/chat/activityV2Store.ts`
- Test: `web/src/stores/__tests__/activityV2.store.spec.ts`

- [ ] **Step 1: Write failing store test**

```typescript
// web/src/stores/__tests__/activityV2.store.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useChatActivityStore } from '../chat/activityV2Store';
import type { Task, Turn, Step } from '../../features/chat/v2Types';

function makeTask(over: Partial<Task> = {}): Task {
  return {
    ID: 't1', SessionID: 's1', UserMessage: 'hi', Status: 'running',
    Seq: 1, Version: 1, CreatedAt: '', UpdatedAt: '', CompletedAt: null,
    ...over,
  };
}

describe('useChatActivityStore', () => {
  beforeEach(() => setActivePinia(createPinia()));

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

  it('upsertStep merges streaming content', () => {
    const s = useChatActivityStore();
    const step: Step = {
      ID: 's1', TurnID: 't1', TaskID: 'tk1', SessionID: 's1',
      SpiritSessionID: 's1', Kind: 'reply', AuthorAgentKey: 'a1',
      Seq: 1, Version: 1, Content: '', Reasoning: '',
      ToolName: '', ToolCallID: '', ToolArgs: null, ToolResult: null,
      ToolDurationMs: 0, ToolErrorCode: '', Status: 'running',
      IsFinal: false, StartedAt: '', CompletedAt: null,
    };
    s.upsertStep(step);
    s.appendStepDelta('s1', 'content', 'hello ');
    s.appendStepDelta('s1', 'content', 'world');
    expect(s.steps.get('s1')?.Content).toBe('hello world');
  });

  it('getSessionTasks returns tasks sorted by seq', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't2', Seq: 2 }));
    s.upsertTask(makeTask({ ID: 't1', Seq: 1 }));
    const tasks = s.getSessionTasks('s1');
    expect(tasks.map(t => t.ID)).toEqual(['t1', 't2']);
  });

  it('clearSession removes entities by spirit session id', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't1', SessionID: 's1' }));
    s.upsertTask(makeTask({ ID: 't2', SessionID: 's2' }));
    s.clearSession('s1');
    expect(s.tasks.has('t1')).toBe(false);
    expect(s.tasks.has('t2')).toBe(true);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/stores/__tests__/activityV2.store.spec.ts`
Expected: FAIL — cannot find module `../chat/activityV2Store`

- [ ] **Step 3: Write the store**

```typescript
// web/src/stores/chat/activityV2Store.ts
import { ref, computed } from 'vue';
import { defineStore } from 'pinia';
import type {
  Task, Turn, Step, TeamStage, TeamRun, MemberSession,
  PlanBoard, PlanStep,
} from '../../features/chat/v2Types';

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

  // === Upsert helpers (optimistic-concurrency guarded) ===

  function upsertTask(t: Task) {
    const ex = tasks.value.get(t.ID);
    if (ex && t.Version <= ex.Version) return;
    tasks.value.set(t.ID, { ...t });
  }

  function upsertTurn(t: Turn) {
    const ex = turns.value.get(t.ID);
    if (ex && t.Version <= ex.Version) return;
    turns.value.set(t.ID, { ...t });
  }

  function upsertStep(s: Step) {
    const ex = steps.value.get(s.ID);
    if (ex && s.Version < ex.Version) return;
    // For same version (streaming updates), merge content fields instead of replacing
    if (ex && s.Version === ex.Version) {
      steps.value.set(s.ID, { ...ex, ...s });
    } else {
      steps.value.set(s.ID, { ...s });
    }
  }

  function upsertTeamStage(ts: TeamStage) {
    const ex = teamStages.value.get(ts.ID);
    if (ex && ts.Version <= ex.Version) return;
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
    memberSessions.value.set(ms.ID, { ...ms });
  }

  function upsertPlanBoard(pb: PlanBoard) {
    const ex = planBoards.value.get(pb.ID);
    if (ex && pb.Version <= ex.Version) return;
    planBoards.value.set(pb.ID, { ...pb });
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

  // === Streaming delta (does NOT bump version) ===

  function appendStepDelta(stepId: string, field: 'content' | 'reasoning', chunk: string) {
    const s = steps.value.get(stepId);
    if (!s) return;
    if (field === 'content') s.Content += chunk;
    else s.Reasoning += chunk;
  }

  // === Query helpers ===

  function getSessionTasks(sessionId: string): Task[] {
    const out: Task[] = [];
    for (const t of tasks.value.values()) {
      if (t.SessionID === sessionId) out.push(t);
    }
    return out.sort((a, b) => a.Seq - b.Seq);
  }

  function getTaskTurns(taskId: string): Turn[] {
    const out: Turn[] = [];
    for (const t of turns.value.values()) {
      if (t.TaskID === taskId) out.push(t);
    }
    return out.sort((a, b) => a.Seq - b.Seq);
  }

  function getTurnSteps(turnId: string): Step[] {
    const out: Step[] = [];
    for (const s of steps.value.values()) {
      if (s.TurnID === turnId) out.push(s);
    }
    return out.sort((a, b) => a.Seq - b.Seq);
  }

  function getTaskTeamStages(taskId: string): TeamStage[] {
    const out: TeamStage[] = [];
    for (const ts of teamStages.value.values()) {
      if (ts.TaskID === taskId) out.push(ts);
    }
    return out.sort((a, b) => a.Seq - b.Seq);
  }

  function getTaskPlanBoards(taskId: string): PlanBoard[] {
    const out: PlanBoard[] = [];
    for (const pb of planBoards.value.values()) {
      if (pb.TaskID === taskId) out.push(pb);
    }
    return out.sort((a, b) => a.Seq - b.Seq);
  }

  function getTeamStageTeamRuns(teamStageId: string): TeamRun[] {
    const out: TeamRun[] = [];
    for (const tr of teamRuns.value.values()) {
      if (tr.TeamStageID === teamStageId) out.push(tr);
    }
    return out.sort((a, b) => a.Seq - b.Seq);
  }

  function getTeamRunMemberSessions(teamRunId: string): MemberSession[] {
    const out: MemberSession[] = [];
    for (const ms of memberSessions.value.values()) {
      if (ms.TeamRunID === teamRunId) out.push(ms);
    }
    return out.sort((a, b) => a.Seq - b.Seq);
  }

  // === Bulk operations ===

  function clearSession(spiritSessionId: string) {
    for (const [id, t] of tasks.value) {
      if (t.SessionID === spiritSessionId) tasks.value.delete(id);
    }
    for (const [id, t] of turns.value) {
      if (t.SpiritSessionID === spiritSessionId) turns.value.delete(id);
    }
    for (const [id, s] of steps.value) {
      if (s.SpiritSessionID === spiritSessionId) steps.value.delete(id);
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
  }

  function clearAll() {
    tasks.value.clear();
    turns.value.clear();
    steps.value.clear();
    teamStages.value.clear();
    teamRuns.value.clear();
    memberSessions.value.clear();
    planBoards.value.clear();
    planSteps.value.clear();
  }

  return {
    tasks, turns, steps, teamStages, teamRuns, memberSessions, planBoards, planSteps,
    upsertTask, upsertTurn, upsertStep, upsertTeamStage, upsertTeamRun,
    upsertMemberSession, upsertPlanBoard, upsertPlanStep,
    appendStepDelta,
    getSessionTasks, getTaskTurns, getTurnSteps,
    getTaskTeamStages, getTaskPlanBoards,
    getTeamStageTeamRuns, getTeamRunMemberSessions,
    clearSession, clearAll,
  };
});
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/stores/__tests__/activityV2.store.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd web && git add src/stores/chat/activityV2Store.ts src/stores/__tests__/activityV2.store.spec.ts
git commit -m "feat(chat/v2): add Pinia store with flat Maps for v2 entities"
```

---

### Task 3: WS Event Router — useChatEventRouter

**Files:**
- Create: `web/src/features/chat/composables/useChatEventRouter.ts`
- Test: `web/src/features/chat/composables/__tests__/useChatEventRouter.spec.ts`

- [ ] **Step 1: Write failing event-router test**

```typescript
// web/src/features/chat/composables/__tests__/useChatEventRouter.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import { useChatEventRouter } from '../useChatEventRouter';
import type { V2WsEnvelope, Task, Step } from '../../v2Types';

function mkTask(over: Partial<Task> = {}): Task {
  return {
    ID: 't1', SessionID: 's1', UserMessage: 'hi', Status: 'running',
    Seq: 1, Version: 1, CreatedAt: '', UpdatedAt: '', CompletedAt: null, ...over,
  };
}

function mkStep(over: Partial<Step> = {}): Step {
  return {
    ID: 's1', TurnID: 't1', TaskID: 'tk1', SessionID: 's1',
    SpiritSessionID: 's1', Kind: 'reply', AuthorAgentKey: 'a1',
    Seq: 1, Version: 1, Content: '', Reasoning: '',
    ToolName: '', ToolCallID: '', ToolArgs: null, ToolResult: null,
    ToolDurationMs: 0, ToolErrorCode: '', Status: 'running',
    IsFinal: false, StartedAt: '', CompletedAt: null, ...over,
  };
}

describe('useChatEventRouter', () => {
  let store: ReturnType<typeof useChatActivityStore>;
  let router: ReturnType<typeof useChatEventRouter>;

  beforeEach(() => {
    setActivePinia(createPinia());
    store = useChatActivityStore();
    router = useChatEventRouter(store);
  });

  it('handles task.created', () => {
    router.dispatch({ type: 'v2_event', kind: 'task.created', payload: { Task: mkTask() } });
    expect(store.tasks.get('t1')?.UserMessage).toBe('hi');
  });

  it('handles step.created', () => {
    router.dispatch({ type: 'v2_event', kind: 'step.created', payload: { Step: mkStep() } });
    expect(store.steps.get('s1')?.Kind).toBe('reply');
  });

  it('handles step.streaming by appending content', () => {
    store.upsertStep(mkStep({ ID: 's1', Content: '' }));
    router.dispatch({
      type: 'v2_event', kind: 'step.streaming',
      payload: { StepID: 's1', DeltaField: 'content', DeltaChunk: 'hello' },
    });
    expect(store.steps.get('s1')?.Content).toBe('hello');
  });

  it('handles step.streaming reasoning', () => {
    store.upsertStep(mkStep({ ID: 's1', Reasoning: '' }));
    router.dispatch({
      type: 'v2_event', kind: 'step.streaming',
      payload: { StepID: 's1', DeltaField: 'reasoning', DeltaChunk: 'think' },
    });
    expect(store.steps.get('s1')?.Reasoning).toBe('think');
  });

  it('handles task.completed with version guard', () => {
    store.upsertTask(mkTask({ ID: 't1', Version: 1, Status: 'running' }));
    router.dispatch({
      type: 'v2_event', kind: 'task.completed',
      payload: { Task: mkTask({ ID: 't1', Version: 2, Status: 'completed' }) },
    });
    expect(store.tasks.get('t1')?.Status).toBe('completed');
  });

  it('ignores unknown event kinds', () => {
    router.dispatch({ type: 'v2_event', kind: 'unknown.kind' as never, payload: {} as never });
    expect(store.tasks.size).toBe(0);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/features/chat/composables/__tests__/useChatEventRouter.spec.ts`
Expected: FAIL — cannot find module `../useChatEventRouter`

- [ ] **Step 3: Write the event router**

```typescript
// web/src/features/chat/composables/useChatEventRouter.ts
import type { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { V2WsEnvelope, EventKind } from '../v2Types';

type Store = ReturnType<typeof useChatActivityStore>;

/**
 * useChatEventRouter dispatches v2 WS events into Pinia store mutations.
 *
 * The router is a pure function of (store, envelope) → store mutation.
 * No inference, no dedup, no re-parenting — just direct Map updates.
 */
export function useChatEventRouter(store: Store) {
  function dispatch(env: V2WsEnvelope) {
    if (env.type !== 'v2_event') return;
    handleKind(env.kind, env.payload);
  }

  function handleKind(kind: EventKind, payload: unknown) {
    const p = payload as Record<string, unknown>;
    switch (kind) {
      // Task events
      case 'task.created':
      case 'task.updated':
      case 'task.completed':
      case 'task.failed':
        if (p.Task) store.upsertTask(p.Task as never);
        break;

      // Turn events
      case 'turn.started':
      case 'turn.completed':
      case 'turn.failed':
        if (p.Turn) store.upsertTurn(p.Turn as never);
        break;

      // Step events
      case 'step.created':
        if (p.Step) store.upsertStep(p.Step as never);
        break;
      case 'step.streaming':
        if (p.StepID && p.DeltaField && p.DeltaChunk) {
          store.appendStepDelta(
            p.StepID as string,
            p.DeltaField as 'content' | 'reasoning',
            p.DeltaChunk as string,
          );
        }
        break;
      case 'step.updated':
      case 'step.completed':
      case 'step.failed':
        if (p.Step) store.upsertStep(p.Step as never);
        break;

      // TeamStage events
      case 'team_stage.created':
      case 'team_stage.updated':
      case 'team_stage.completed':
      case 'team_stage.failed':
        if (p.TeamStage) store.upsertTeamStage(p.TeamStage as never);
        break;

      // TeamRun events
      case 'team_run.started':
      case 'team_run.completed':
      case 'team_run.failed':
        if (p.TeamRun) store.upsertTeamRun(p.TeamRun as never);
        break;

      // MemberSession events
      case 'member_session.created':
      case 'member_session.updated':
        if (p.MemberSession) store.upsertMemberSession(p.MemberSession as never);
        break;

      // PlanBoard events
      case 'plan_board.created':
      case 'plan_board.updated':
        if (p.PlanBoard) store.upsertPlanBoard(p.PlanBoard as never);
        break;

      // PlanStep events
      case 'plan_step.started':
      case 'plan_step.completed':
      case 'plan_step.failed':
      case 'plan_step.updated':
        if (p.PlanStep) store.upsertPlanStep(p.PlanStep as never);
        break;
      case 'plan_step.skipped':
        if (p.PlanStep) store.upsertPlanStep(p.PlanStep as never);
        break;

      default:
        // Unknown event kind — silently ignore (forward compatibility)
        break;
    }
  }

  return { dispatch };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/features/chat/composables/__tests__/useChatEventRouter.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd web && git add src/features/chat/composables/useChatEventRouter.ts src/features/chat/composables/__tests__/useChatEventRouter.spec.ts
git commit -m "feat(chat/v2): add WS event router dispatching v2 events to store"
```

---

### Task 4: WS Transport — Handle v2_event Envelope

**Files:**
- Modify: `web/src/realtime/ws-transport.ts`
- Modify: `web/src/realtime/globalWsHub.ts`
- Test: `web/src/realtime/__tests__/ws-transport-v2.spec.ts`

- [ ] **Step 1: Write failing transport test**

```typescript
// web/src/realtime/__tests__/ws-transport-v2.spec.ts
import { describe, it, expect, vi } from 'vitest';
import { createWsTransport } from '../ws-transport';

// Minimal WebSocket mock
class MockWS {
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: ((e: unknown) => void) | null = null;
  readyState = 1;
  send() {}
  close() {}
}

describe('ws-transport v2_event', () => {
  it('dispatches v2_event to onV2Event callback', () => {
    const mock = new MockWS();
    const onV2Event = vi.fn();
    const transport = createWsTransport({
      sessionId: 's1',
      url: 'ws://localhost',
      socketFactory: () => mock as unknown as WebSocket,
      onV2Event,
    });
    transport.connect();

    const v2Msg = JSON.stringify({
      type: 'v2_event',
      kind: 'task.created',
      payload: { Task: { ID: 't1' } },
    });
    mock.onmessage!({ data: v2Msg });

    expect(onV2Event).toHaveBeenCalledTimes(1);
    const arg = onV2Event.mock.calls[0][0];
    expect(arg.type).toBe('v2_event');
    expect(arg.kind).toBe('task.created');
    transport.disconnect();
  });

  it('does NOT dispatch v2_event to onActivityEvent', () => {
    const mock = new MockWS();
    const onActivityEvent = vi.fn();
    const onV2Event = vi.fn();
    const transport = createWsTransport({
      sessionId: 's1',
      url: 'ws://localhost',
      socketFactory: () => mock as unknown as WebSocket,
      onActivityEvent,
      onV2Event,
    });
    transport.connect();

    const v2Msg = JSON.stringify({ type: 'v2_event', kind: 'task.created', payload: {} });
    mock.onmessage!({ data: v2Msg });

    expect(onActivityEvent).not.toHaveBeenCalled();
    expect(onV2Event).toHaveBeenCalledTimes(1);
    transport.disconnect();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/realtime/__tests__/ws-transport-v2.spec.ts`
Expected: FAIL — `onV2Event` not a recognized option; v2 events dropped by `direction` check

- [ ] **Step 3: Modify ws-transport.ts to handle v2 events**

In `web/src/realtime/ws-transport.ts`, find the `onmessage` handler (around line 147). Add v2_event dispatch BEFORE the `direction` check:

```typescript
// --- ADD this block BEFORE the existing `const msg = JSON.parse(...)` line ---
// Actually, modify the existing onmessage handler:

ws.onmessage = (ev: MessageEvent) => {
  try {
    const raw = JSON.parse(ev.data) as Record<string, unknown>;

    // v2 events: envelope { type: "v2_event", kind, payload } — no direction field
    if (raw.type === 'v2_event') {
      opts.onV2Event?.(raw as unknown as V2WsEnvelope);
      return;
    }

    const msg = raw as WsDownstream;
    if (msg.direction !== 'server_to_client') return;

    // ... rest of existing handler (connected, pong, server_shutdown, activity_event, monitor_event)
```

Also add `onV2Event` to the `WsTransportOptions` interface and import `V2WsEnvelope`:

```typescript
import type { V2WsEnvelope } from '../features/chat/v2Types';

export interface WsTransportOptions {
  // ... existing fields ...
  onV2Event?: (envelope: V2WsEnvelope) => void;
}
```

- [ ] **Step 4: Modify globalWsHub.ts to forward v2 events**

In `web/src/realtime/globalWsHub.ts`, add `onV2Event` to the consumer options and dispatch:

```typescript
// Add to GlobalWsConsumerOptions interface:
onV2Event?: (envelope: V2WsEnvelope) => void;

// In acquireGlobalWsConsumer, add onV2Event to the transport options:
const transport = createWsTransport({
  // ... existing options ...
  onV2Event: (env) => {
    consumers.forEach(c => c.onV2Event?.(env));
  },
});
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd web && npx vitest run src/realtime/__tests__/ws-transport-v2.spec.ts`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd web && git add src/realtime/ws-transport.ts src/realtime/globalWsHub.ts src/realtime/__tests__/ws-transport-v2.spec.ts
git commit -m "feat(realtime): handle v2_event WS envelope in transport and global hub"
```

---

## Tier 1: Core Rendering — Tree Builder + Step/Turn/Task Components

### Task 5: Tree Builder — useTaskTree

**Files:**
- Create: `web/src/features/chat/composables/useTaskTree.ts`
- Test: `web/src/features/chat/composables/__tests__/useTaskTree.spec.ts`

- [ ] **Step 1: Write failing tree-builder test**

```typescript
// web/src/features/chat/composables/__tests__/useTaskTree.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import { useTaskTree } from '../useTaskTree';
import type { Task, Turn, Step } from '../../v2Types';

describe('useTaskTree', () => {
  let store: ReturnType<typeof useChatActivityStore>;

  beforeEach(() => {
    setActivePinia(createPinia());
    store = useChatActivityStore();
  });

  it('builds empty tree for empty store', () => {
    const { buildTaskTree } = useTaskTree(store);
    const tree = buildTaskTree('tk1');
    expect(tree).toBeNull();
  });

  it('builds tree with task + turns + steps sorted by seq', () => {
    store.upsertTask({ ID: 'tk1', SessionID: 's1', UserMessage: 'hi', Status: 'completed', Seq: 1, Version: 1, CreatedAt: '', UpdatedAt: '', CompletedAt: null } as Task);
    store.upsertTurn({ ID: 'turn2', TaskID: 'tk1', SessionID: 's1', SpiritSessionID: 's1', ParentTurnID: '', AgentKey: 'a1', TeamID: '', TeamStageID: '', Seq: 2, Version: 1, Status: 'completed', StartedAt: '', CompletedAt: null } as never);
    store.upsertTurn({ ID: 'turn1', TaskID: 'tk1', SessionID: 's1', SpiritSessionID: 's1', ParentTurnID: '', AgentKey: 'a1', TeamID: '', TeamStageID: '', Seq: 1, Version: 1, Status: 'completed', StartedAt: '', CompletedAt: null } as never);
    store.upsertStep({ ID: 's2', TurnID: 'turn1', TaskID: 'tk1', SessionID: 's1', SpiritSessionID: 's1', Kind: 'reply', AuthorAgentKey: 'a1', Seq: 2, Version: 1, Content: 'world', Reasoning: '', ToolName: '', ToolCallID: '', ToolArgs: null, ToolResult: null, ToolDurationMs: 0, ToolErrorCode: '', Status: 'completed', IsFinal: true, StartedAt: '', CompletedAt: null } as never);
    store.upsertStep({ ID: 's1', TurnID: 'turn1', TaskID: 'tk1', SessionID: 's1', SpiritSessionID: 's1', Kind: 'thinking', AuthorAgentKey: 'a1', Seq: 1, Version: 1, Content: '', Reasoning: 'think', ToolName: '', ToolCallID: '', ToolArgs: null, ToolResult: null, ToolDurationMs: 0, ToolErrorCode: '', Status: 'completed', IsFinal: false, StartedAt: '', CompletedAt: null } as never);

    const { buildTaskTree } = useTaskTree(store);
    const tree = buildTaskTree('tk1');
    expect(tree?.task.ID).toBe('tk1');
    expect(tree?.turnTrees.map(t => t.turn.ID)).toEqual(['turn1', 'turn2']);
    expect(tree?.turnTrees[0].steps.map(s => s.ID)).toEqual(['s1', 's2']);
  });

  it('includes team stages and plan boards for task', () => {
    store.upsertTask({ ID: 'tk1', SessionID: 's1', UserMessage: 'hi', Status: 'running', Seq: 1, Version: 1, CreatedAt: '', UpdatedAt: '', CompletedAt: null } as Task);
    store.upsertTeamStage({ ID: 'ts1', TaskID: 'tk1', TurnID: '', SessionID: 's1', TeamID: 'team1', DagNodeID: '', DependsOn: [], Status: 'running', Stage: 'executing', Members: [], Strategy: 'parallel', StartedAt: '', CompletedAt: null, Seq: 1, Version: 1 } as never);
    store.upsertPlanBoard({ ID: 'pb1', TaskID: 'tk1', TurnID: '', SessionID: 's1', Strategy: 'dag', Status: 'executing', Steps: [], StartedAt: '', CompletedAt: null, Seq: 1, Version: 1 } as never);

    const { buildTaskTree } = useTaskTree(store);
    const tree = buildTaskTree('tk1');
    expect(tree?.teamStages.map(ts => ts.ID)).toEqual(['ts1']);
    expect(tree?.planBoards.map(pb => pb.ID)).toEqual(['pb1']);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/features/chat/composables/__tests__/useTaskTree.spec.ts`
Expected: FAIL — cannot find module `../useTaskTree`

- [ ] **Step 3: Write the tree builder**

```typescript
// web/src/features/chat/composables/useTaskTree.ts
import { computed } from 'vue';
import type { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Task, Turn, Step, TeamStage, PlanBoard } from '../v2Types';

type Store = ReturnType<typeof useChatActivityStore>;

export interface TurnTree {
  turn: Turn;
  steps: Step[];
}

export interface TaskTree {
  task: Task;
  turnTrees: TurnTree[];
  teamStages: TeamStage[];
  planBoards: PlanBoard[];
}

/**
 * useTaskTree builds a TaskTree from the Pinia store by filtering
 * entities by task_id and sorting by seq. No inference, no re-parenting.
 */
export function useTaskTree(store: Store) {
  function buildTaskTree(taskId: string): TaskTree | null {
    const task = store.tasks.get(taskId);
    if (!task) return null;

    const turns = store.getTaskTurns(taskId);
    const turnTrees: TurnTree[] = turns.map(turn => ({
      turn,
      steps: store.getTurnSteps(turn.ID),
    }));

    const teamStages = store.getTaskTeamStages(taskId);
    const planBoards = store.getTaskPlanBoards(taskId);

    return { task, turnTrees, teamStages, planBoards };
  }

  return { buildTaskTree };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/features/chat/composables/__tests__/useTaskTree.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd web && git add src/features/chat/composables/useTaskTree.ts src/features/chat/composables/__tests__/useTaskTree.spec.ts
git commit -m "feat(chat/v2): add useTaskTree composable for seq-sorted tree building"
```

---

### Task 6: Adapt Step Block Components (ThinkingBlock, ActionBlock, ReplyBlock)

**Files:**
- Modify: `web/src/components/chat/ThinkingBlock.vue`
- Modify: `web/src/components/chat/ActionBlock.vue`
- Modify: `web/src/components/chat/ReplyBlock.vue`
- Test: `web/src/components/chat/__tests__/StepBlocks.spec.ts`

These components currently take `ThinkingEvent` / `ActionEvent` / `ReplyEvent` from `streamEventTypes.ts`. They need to accept a `Step` from `v2Types.ts` instead. The internal rendering (Markdown, collapse, tool detail) stays the same — only the props adapter changes.

- [ ] **Step 1: Write failing component test**

```typescript
// web/src/components/chat/__tests__/StepBlocks.spec.ts
import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import ThinkingBlock from '../ThinkingBlock.vue';
import ReplyBlock from '../ReplyBlock.vue';
import type { Step } from '../../../features/chat/v2Types';

function mkStep(over: Partial<Step> = {}): Step {
  return {
    ID: 's1', TurnID: 't1', TaskID: 'tk1', SessionID: 's1',
    SpiritSessionID: 's1', Kind: 'thinking', AuthorAgentKey: 'a1',
    Seq: 1, Version: 1, Content: '', Reasoning: 'I should help',
    ToolName: '', ToolCallID: '', ToolArgs: null, ToolResult: null,
    ToolDurationMs: 0, ToolErrorCode: '', Status: 'completed',
    IsFinal: false, StartedAt: '', CompletedAt: null, ...over,
  };
}

describe('ThinkingBlock v2', () => {
  it('accepts Step prop', () => {
    const wrapper = mount(ThinkingBlock, {
      props: { step: mkStep({ Kind: 'thinking', Reasoning: 'test reasoning' }) },
    });
    expect(wrapper.text()).toContain('test reasoning');
  });
});

describe('ReplyBlock v2', () => {
  it('accepts Step prop with content', () => {
    const wrapper = mount(ReplyBlock, {
      props: { step: mkStep({ Kind: 'reply', Content: 'Hello world', IsFinal: true }) },
    });
    expect(wrapper.text()).toContain('Hello world');
  });

  it('shows final label when IsFinal', () => {
    const wrapper = mount(ReplyBlock, {
      props: { step: mkStep({ Content: 'hi', IsFinal: true }) },
    });
    expect(wrapper.text()).toContain('最终回复');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/components/chat/__tests__/StepBlocks.spec.ts`
Expected: FAIL — components don't accept `step` prop yet

- [ ] **Step 3: Modify ThinkingBlock.vue props**

In `web/src/components/chat/ThinkingBlock.vue`, change the props definition:

**Old:**
```typescript
defineProps<{
  messageId: string;
  reasoning: string;
  streaming?: boolean;
  thinkingOnly?: boolean;
  durationMs?: number | null;
  isDark?: boolean;
  defaultCollapsed?: boolean;
  label?: string;
}>()
```

**New:**
```typescript
import type { Step } from '../../features/chat/v2Types';

const props = defineProps<{
  step: Step;
  isDark?: boolean;
  defaultCollapsed?: boolean;
  label?: string;
}>()

// Derived values (backward-compatible with internal template)
const messageId = computed(() => props.step.ID);
const reasoning = computed(() => props.step.Reasoning);
const streaming = computed(() => props.step.Status === 'running');
const durationMs = computed(() => props.step.CompletedAt ? null : null);
const thinkingOnly = computed(() => !props.step.Reasoning && props.step.Status === 'running');
```

Keep the rest of the template and logic unchanged. Add `import { computed } from 'vue'` if not already present.

- [ ] **Step 4: Modify ActionBlock.vue props**

In `web/src/components/chat/ActionBlock.vue`, change props to accept `Step`:

```typescript
import type { Step } from '../../features/chat/v2Types';

const props = defineProps<{
  step: Step;
  agentColor?: string;
}>()

// Bridge to internal logic that previously used activity.tool.* fields
const activity = computed(() => ({
  id: props.step.ID,
  tool: {
    name: props.step.ToolName,
    toolCategory: deriveToolCategory(props.step.ToolName),
    arguments: props.step.ToolArgs,
    result: props.step.ToolResult,
    durationMs: props.step.ToolDurationMs,
    error: props.step.ToolErrorCode,
  },
  status: props.step.Status,
}));
```

Keep the `deriveToolCategory` helper and all internal rendering unchanged.

- [ ] **Step 5: Modify ReplyBlock.vue props**

In `web/src/components/chat/ReplyBlock.vue`, change props:

```typescript
import type { Step } from '../../features/chat/v2Types';

const props = defineProps<{
  step: Step;
}>()

const content = computed(() => props.step.Content);
const streaming = computed(() => props.step.Status === 'running');
const isFinal = computed(() => props.step.IsFinal);
const messageId = computed(() => props.step.ID);
```

Keep the template logic the same, using these computed refs.

- [ ] **Step 6: Run test to verify it passes**

Run: `cd web && npx vitest run src/components/chat/__tests__/StepBlocks.spec.ts`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
cd web && git add src/components/chat/ThinkingBlock.vue src/components/chat/ActionBlock.vue src/components/chat/ReplyBlock.vue src/components/chat/__tests__/StepBlocks.spec.ts
git commit -m "feat(chat/v2): adapt ThinkingBlock/ActionBlock/ReplyBlock to accept Step prop"
```

---

### Task 7: Core Container Components (TurnContainer, TurnList, TaskCard, TaskList, SessionPanel)

**Files:**
- Create: `web/src/components/chat/v2/TurnContainer.vue`
- Create: `web/src/components/chat/v2/TurnList.vue`
- Create: `web/src/components/chat/v2/TaskCard.vue`
- Create: `web/src/components/chat/v2/TaskList.vue`
- Create: `web/src/components/chat/v2/SessionPanel.vue`
- Test: `web/src/components/chat/v2/__tests__/CoreContainers.spec.ts`

- [ ] **Step 1: Write failing test**

```typescript
// web/src/components/chat/v2/__tests__/CoreContainers.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import SessionPanel from '../SessionPanel.vue';
import TaskList from '../TaskList.vue';
import TaskCard from '../TaskCard.vue';
import TurnList from '../TurnList.vue';
import TurnContainer from '../TurnContainer.vue';
import type { Task, Turn, Step } from '../../../features/chat/v2Types';

describe('v2 Core Containers', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('TaskCard renders user message', () => {
    const wrapper = mount(TaskCard, {
      props: { task: { ID: 'tk1', SessionID: 's1', UserMessage: 'Hello', Status: 'completed', Seq: 1, Version: 1, CreatedAt: '', UpdatedAt: '', CompletedAt: null } as Task },
    });
    expect(wrapper.text()).toContain('Hello');
  });

  it('TurnContainer renders thinking + reply steps', async () => {
    const store = useChatActivityStore();
    store.upsertStep({ ID: 's1', TurnID: 'turn1', TaskID: 'tk1', SessionID: 's1', SpiritSessionID: 's1', Kind: 'thinking', AuthorAgentKey: 'a1', Seq: 1, Version: 1, Content: '', Reasoning: 'think', ToolName: '', ToolCallID: '', ToolArgs: null, ToolResult: null, ToolDurationMs: 0, ToolErrorCode: '', Status: 'completed', IsFinal: false, StartedAt: '', CompletedAt: null } as Step);
    store.upsertStep({ ID: 's2', TurnID: 'turn1', TaskID: 'tk1', SessionID: 's1', SpiritSessionID: 's1', Kind: 'reply', AuthorAgentKey: 'a1', Seq: 2, Version: 1, Content: 'reply text', Reasoning: '', ToolName: '', ToolCallID: '', ToolArgs: null, ToolResult: null, ToolDurationMs: 0, ToolErrorCode: '', Status: 'completed', IsFinal: true, StartedAt: '', CompletedAt: null } as Step);
    const wrapper = mount(TurnContainer, {
      props: { turn: { ID: 'turn1', TaskID: 'tk1', SessionID: 's1', SpiritSessionID: 's1', ParentTurnID: '', AgentKey: 'a1', TeamID: '', TeamStageID: '', Seq: 1, Version: 1, Status: 'completed', StartedAt: '', CompletedAt: null } as Turn },
      global: { plugins: [createPinia()] },
    });
    expect(wrapper.text()).toContain('think');
    expect(wrapper.text()).toContain('reply text');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/components/chat/v2/__tests__/CoreContainers.spec.ts`
Expected: FAIL — components don't exist

- [ ] **Step 3: Create TurnContainer.vue**

```vue
<!-- web/src/components/chat/v2/TurnContainer.vue -->
<template>
  <div class="turn-container" :data-turn-id="turn.ID">
    <template v-for="step in steps" :key="step.ID">
      <ThinkingBlock v-if="step.Kind === 'thinking'" :step="step" />
      <ActionBlock v-else-if="step.Kind === 'action'" :step="step" />
      <ReplyBlock v-else-if="step.Kind === 'reply'" :step="step" />
      <NoticeBlock v-else-if="step.Kind === 'notice'" :step="step" />
      <ConfirmBlock v-else-if="step.Kind === 'confirm'" :step="step" />
      <ErrorBlock v-else-if="step.Kind === 'error'" :step="step" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Turn } from '../../../features/chat/v2Types';
import ThinkingBlock from '../ThinkingBlock.vue';
import ActionBlock from '../ActionBlock.vue';
import ReplyBlock from '../ReplyBlock.vue';
// NoticeBlock, ConfirmBlock, ErrorBlock are existing components — adapt later or reuse
import NoticeBlock from '../NoticeBlock.vue';
import ConfirmBlock from '../ConfirmBlock.vue';
import ErrorBlock from '../ErrorBlock.vue';

const props = defineProps<{ turn: Turn }>();
const store = useChatActivityStore();
const steps = computed(() => store.getTurnSteps(props.turn.ID));
</script>
```

- [ ] **Step 4: Create TurnList.vue**

```vue
<!-- web/src/components/chat/v2/TurnList.vue -->
<template>
  <div class="turn-list">
    <TurnContainer v-for="turn in turns" :key="turn.ID" :turn="turn" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Turn } from '../../../features/chat/v2Types';
import TurnContainer from './TurnContainer.vue';

const props = defineProps<{ turns: Turn[] }>();
</script>
```

- [ ] **Step 5: Create TaskCard.vue**

```vue
<!-- web/src/components/chat/v2/TaskCard.vue -->
<template>
  <div class="task-card" :data-task-id="task.ID">
    <div class="task-user-message">{{ task.UserMessage }}</div>
    <div v-if="task.Status === 'running'" class="task-status">处理中...</div>
    <TurnList :turns="turns" />
    <TeamStagePanel v-for="ts in teamStages" :key="ts.ID" :team-stage="ts" />
    <PlanBoardCard v-for="pb in planBoards" :key="pb.ID" :plan-board="pb" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Task } from '../../../features/chat/v2Types';
import TurnList from './TurnList.vue';
import TeamStagePanel from './TeamStagePanel.vue';
import PlanBoardCard from './PlanBoardCard.vue';

const props = defineProps<{ task: Task }>();
const store = useChatActivityStore();
const turns = computed(() => store.getTaskTurns(props.task.ID));
const teamStages = computed(() => store.getTaskTeamStages(props.task.ID));
const planBoards = computed(() => store.getTaskPlanBoards(props.task.ID));
</script>
```

- [ ] **Step 6: Create TaskList.vue**

```vue
<!-- web/src/components/chat/v2/TaskList.vue -->
<template>
  <div class="task-list">
    <TaskCard v-for="task in tasks" :key="task.ID" :task="task" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import TaskCard from './TaskCard.vue';

const props = defineProps<{ sessionId: string }>();
const store = useChatActivityStore();
const tasks = computed(() => store.getSessionTasks(props.sessionId));
</script>
```

- [ ] **Step 7: Create SessionPanel.vue**

```vue
<!-- web/src/components/chat/v2/SessionPanel.vue -->
<template>
  <div class="session-panel">
    <TaskList :session-id="sessionId" />
  </div>
</template>

<script setup lang="ts">
import TaskList from './TaskList.vue';

defineProps<{ sessionId: string }>();
</script>
```

- [ ] **Step 8: Run test to verify it passes**

Run: `cd web && npx vitest run src/components/chat/v2/__tests__/CoreContainers.spec.ts`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
cd web && git add src/components/chat/v2/
git commit -m "feat(chat/v2): add core container components (TurnContainer/TurnList/TaskCard/TaskList/SessionPanel)"
```

---

## Tier 2: Team Rendering

### Task 8: Team Components (MemberSessionPanel, TeamRunCard, TeamStagePanel)

**Files:**
- Create: `web/src/components/chat/v2/MemberSessionPanel.vue`
- Create: `web/src/components/chat/v2/TeamRunCard.vue`
- Create: `web/src/components/chat/v2/TeamStagePanel.vue`
- Test: `web/src/components/chat/v2/__tests__/TeamComponents.spec.ts`

- [ ] **Step 1: Write failing test**

```typescript
// web/src/components/chat/v2/__tests__/TeamComponents.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import MemberSessionPanel from '../MemberSessionPanel.vue';
import TeamRunCard from '../TeamRunCard.vue';
import TeamStagePanel from '../TeamStagePanel.vue';
import type { MemberSession, TeamRun, TeamStage } from '../../../features/chat/v2Types';

describe('v2 Team Components', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('MemberSessionPanel renders agent name', () => {
    const wrapper = mount(MemberSessionPanel, {
      props: {
        memberSession: {
          ID: 'ms1', TeamRunID: 'tr1', TeamStageID: 'ts1', TaskID: 'tk1',
          SessionID: 'ms-sess', SpiritSessionID: 's1', AgentKey: 'coder',
          AgentName: 'Coder', AvatarURL: '', Status: 'running', Seq: 1, Version: 1,
          StartedAt: '', FinishedAt: null, Error: '',
        } as MemberSession,
      },
    });
    expect(wrapper.text()).toContain('Coder');
  });

  it('TeamRunCard renders member sessions', async () => {
    const store = useChatActivityStore();
    store.upsertMemberSession({
      ID: 'ms1', TeamRunID: 'tr1', TeamStageID: 'ts1', TaskID: 'tk1',
      SessionID: 'ms-sess', SpiritSessionID: 's1', AgentKey: 'coder',
      AgentName: 'Coder', AvatarURL: '', Status: 'completed', Seq: 1, Version: 1,
      StartedAt: '', FinishedAt: '', Error: '',
    } as MemberSession);
    const wrapper = mount(TeamRunCard, {
      props: {
        teamRun: {
          ID: 'tr1', TeamStageID: 'ts1', TaskID: 'tk1', SessionID: 's1',
          SpiritSessionID: 's1', DagNodeID: '', DependsOn: [], Status: 'completed',
          StartedAt: '', CompletedAt: '', Seq: 1, Version: 1, Error: '',
        } as TeamRun,
      },
      global: { plugins: [createPinia()] },
    });
    expect(wrapper.text()).toContain('Coder');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/components/chat/v2/__tests__/TeamComponents.spec.ts`
Expected: FAIL — components don't exist

- [ ] **Step 3: Create MemberSessionPanel.vue**

```vue
<!-- web/src/components/chat/v2/MemberSessionPanel.vue -->
<template>
  <div class="member-session-panel" :data-agent-key="memberSession.AgentKey">
    <div class="member-header">
      <q-avatar v-if="memberSession.AvatarURL" :src="memberSession.AvatarURL" size="32px" />
      <span class="member-name">{{ memberSession.AgentName }}</span>
      <q-badge v-if="memberSession.Status === 'running'" color="blue" label="执行中" />
      <q-badge v-else-if="memberSession.Status === 'completed'" color="green" label="完成" />
      <q-badge v-else-if="memberSession.Status === 'failed'" color="red" label="失败" />
    </div>
    <div v-if="memberSession.Error" class="member-error">{{ memberSession.Error }}</div>
  </div>
</template>

<script setup lang="ts">
import type { MemberSession } from '../../../features/chat/v2Types';
defineProps<{ memberSession: MemberSession }>();
</script>
```

- [ ] **Step 4: Create TeamRunCard.vue**

```vue
<!-- web/src/components/chat/v2/TeamRunCard.vue -->
<template>
  <div class="team-run-card" :data-team-run-id="teamRun.ID">
    <div class="team-run-header">
      <q-badge :color="statusColor">{{ teamRun.Status }}</q-badge>
    </div>
    <MemberSessionPanel
      v-for="ms in memberSessions"
      :key="ms.ID"
      :member-session="ms"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { TeamRun } from '../../../features/chat/v2Types';
import MemberSessionPanel from './MemberSessionPanel.vue';

const props = defineProps<{ teamRun: TeamRun }>();
const store = useChatActivityStore();
const memberSessions = computed(() => store.getTeamRunMemberSessions(props.teamRun.ID));
const statusColor = computed(() => ({
  running: 'blue', completed: 'green', failed: 'red', cancelled: 'grey',
}[props.teamRun.Status] || 'grey'));
</script>
```

- [ ] **Step 5: Create TeamStagePanel.vue**

```vue
<!-- web/src/components/chat/v2/TeamStagePanel.vue -->
<template>
  <div class="team-stage-panel" :data-team-stage-id="teamStage.ID">
    <div class="team-stage-header">
      <span>Team: {{ teamStage.TeamID }}</span>
      <q-badge :color="stageColor">{{ teamStage.Stage }}</q-badge>
    </div>
    <div class="team-members">
      <span v-for="m in teamStage.Members" :key="m.AgentKey" class="member-chip">
        {{ m.AgentName }}
      </span>
    </div>
    <TeamRunCard v-for="tr in teamRuns" :key="tr.ID" :team-run="tr" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { TeamStage } from '../../../features/chat/v2Types';
import TeamRunCard from './TeamRunCard.vue';

const props = defineProps<{ teamStage: TeamStage }>();
const store = useChatActivityStore();
const teamRuns = computed(() => store.getTeamStageTeamRuns(props.teamStage.ID));
const stageColor = computed(() => ({
  assembled: 'grey', planning: 'orange', executing: 'blue',
  completed: 'green', failed: 'red',
}[props.teamStage.Stage] || 'grey'));
</script>
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd web && npx vitest run src/components/chat/v2/__tests__/TeamComponents.spec.ts`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
cd web && git add src/components/chat/v2/MemberSessionPanel.vue src/components/chat/v2/TeamRunCard.vue src/components/chat/v2/TeamStagePanel.vue src/components/chat/v2/__tests__/TeamComponents.spec.ts
git commit -m "feat(chat/v2): add team rendering components (MemberSessionPanel/TeamRunCard/TeamStagePanel)"
```

---

## Tier 3: Plan Rendering

### Task 9: Plan DAG Layout + Components

**Files:**
- Create: `web/src/features/chat/composables/usePlanDAGLayout.ts`
- Create: `web/src/components/chat/v2/PlanStepNode.vue`
- Create: `web/src/components/chat/v2/PlanDAG.vue`
- Create: `web/src/components/chat/v2/PlanStepDetailPanel.vue`
- Create: `web/src/components/chat/v2/PlanBoardCard.vue`
- Test: `web/src/features/chat/composables/__tests__/usePlanDAGLayout.spec.ts`

- [ ] **Step 1: Write failing DAG layout test**

```typescript
// web/src/features/chat/composables/__tests__/usePlanDAGLayout.spec.ts
import { describe, it, expect } from 'vitest';
import { usePlanDAGLayout } from '../usePlanDAGLayout';
import type { PlanStep } from '../../v2Types';

function mkStep(id: string, deps: string[] = [], over: Partial<PlanStep> = {}): PlanStep {
  return {
    ID: id, PlanID: 'pb1', TaskID: 'tk1', Label: id, Description: '',
    DependsOn: deps, MappedTeamStageID: '', Status: 'pending', AutoSynthesis: false,
    StartedAt: '', CompletedAt: null, Seq: 1, Version: 1, Result: null, Error: null, ...over,
  };
}

describe('usePlanDAGLayout', () => {
  it('lays out sequential steps in a single column', () => {
    const { layoutDAG } = usePlanDAGLayout();
    const steps = [mkStep('a'), mkStep('b', ['a']), mkStep('c', ['b'])];
    const positions = layoutDAG(steps, { width: 600, nodeWidth: 120, nodeHeight: 60, gapX: 40, gapY: 30 });
    expect(positions.get('a')?.y).toBe(0);
    expect(positions.get('b')?.y).toBeGreaterThan(0);
    expect(positions.get('c')?.y).toBeGreaterThan(positions.get('b')!.y);
  });

  it('lays out parallel steps side by side', () => {
    const { layoutDAG } = usePlanDAGLayout();
    const steps = [mkStep('a'), mkStep('b', ['a']), mkStep('c', ['a']), mkStep('d', ['b', 'c'])];
    const positions = layoutDAG(steps, { width: 600, nodeWidth: 120, nodeHeight: 60, gapX: 40, gapY: 30 });
    // b and c should be at the same y level
    expect(positions.get('b')?.y).toBe(positions.get('c')?.y);
    // b and c should be at different x positions
    expect(positions.get('b')?.x).not.toBe(positions.get('c')?.x);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/features/chat/composables/__tests__/usePlanDAGLayout.spec.ts`
Expected: FAIL — module not found

- [ ] **Step 3: Write usePlanDAGLayout.ts**

```typescript
// web/src/features/chat/composables/usePlanDAGLayout.ts
import type { PlanStep } from '../v2Types';

export interface DAGLayoutOptions {
  width: number;
  nodeWidth: number;
  nodeHeight: number;
  gapX: number;
  gapY: number;
}

export interface NodePosition {
  x: number;
  y: number;
}

/**
 * usePlanDAGLayout computes (x, y) positions for plan step nodes in a
 * top-down DAG layout using longest-path layering.
 */
export function usePlanDAGLayout() {
  function layoutDAG(steps: PlanStep[], opts: DAGLayoutOptions): Map<string, NodePosition> {
    const positions = new Map<string, NodePosition>();
    if (steps.length === 0) return positions;

    // Build dependency graph
    const stepMap = new Map(steps.map(s => [s.ID, s]));
    const layer = new Map<string, number>(); // step ID → layer (0 = root)

    // Compute layer = longest path from any root
    function getLayer(id: string): number {
      if (layer.has(id)) return layer.get(id)!;
      const s = stepMap.get(id);
      if (!s || s.DependsOn.length === 0) {
        layer.set(id, 0);
        return 0;
      }
      const maxDepLayer = Math.max(...s.DependsOn.map(d => getLayer(d)));
      const l = maxDepLayer + 1;
      layer.set(id, l);
      return l;
    }
    steps.forEach(s => getLayer(s.ID));

    // Group by layer
    const byLayer = new Map<number, string[]>();
    for (const [id, l] of layer) {
      if (!byLayer.has(l)) byLayer.set(l, []);
      byLayer.get(l)!.push(id);
    }

    // Position: y = layer * (nodeHeight + gapY), x = centered in layer
    const maxLayer = Math.max(...layer.values());
    for (let l = 0; l <= maxLayer; l++) {
      const ids = byLayer.get(l) || [];
      const layerWidth = ids.length * opts.nodeWidth + (ids.length - 1) * opts.gapX;
      const startX = (opts.width - layerWidth) / 2;
      ids.forEach((id, i) => {
        positions.set(id, {
          x: startX + i * (opts.nodeWidth + opts.gapX),
          y: l * (opts.nodeHeight + opts.gapY),
        });
      });
    }

    return positions;
  }

  return { layoutDAG };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/features/chat/composables/__tests__/usePlanDAGLayout.spec.ts`
Expected: PASS

- [ ] **Step 5: Create PlanStepNode.vue**

```vue
<!-- web/src/components/chat/v2/PlanStepNode.vue -->
<template>
  <g :transform="`translate(${pos.x}, ${pos.y})`" @click="$emit('select', step.ID)">
    <rect
      :width="nodeWidth" :height="nodeHeight" rx="8"
      :fill="statusColor" :stroke="isSelected ? '#1976d2' : '#ccc'" :stroke-width="isSelected ? 2 : 1"
      class="plan-step-node"
    />
    <text :x="nodeWidth / 2" :y="20" text-anchor="middle" fill="white" font-size="12">
      {{ step.Label }}
    </text>
    <text :x="nodeWidth / 2" :y="40" text-anchor="middle" fill="white" font-size="10">
      {{ step.Status }}
    </text>
  </g>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { PlanStep } from '../../../features/chat/v2Types';
import type { NodePosition } from '../../../features/chat/composables/usePlanDAGLayout';

const props = defineProps<{
  step: PlanStep;
  pos: NodePosition;
  nodeWidth: number;
  nodeHeight: number;
  isSelected?: boolean;
}>();

defineEmits<{ select: [id: string] }>();

const statusColor = computed(() => ({
  pending: '#9e9e9e', running: '#2196f3', completed: '#4caf50',
  failed: '#f44336', skipped: '#ff9800', partial_failure: '#ff5722',
}[props.step.Status] || '#9e9e9e'));
</script>
```

- [ ] **Step 6: Create PlanDAG.vue**

```vue
<!-- web/src/components/chat/v2/PlanDAG.vue -->
<template>
  <svg :width="width" :height="height" class="plan-dag">
    <!-- Dependency edges -->
    <line
      v-for="edge in edges" :key="`${edge.from}-${edge.to}`"
      :x1="edge.x1" :y1="edge.y1" :x2="edge.x2" :y2="edge.y2"
      stroke="#bbb" stroke-width="1.5" marker-end="url(#arrowhead)"
    />
    <!-- Nodes -->
    <PlanStepNode
      v-for="step in steps" :key="step.ID"
      :step="step" :pos="positions.get(step.ID) || { x: 0, y: 0 }"
      :node-width="nodeWidth" :node-height="nodeHeight"
      :is-selected="selectedId === step.ID"
      @select="selectedId = $event"
    />
    <defs>
      <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
        <polygon points="0 0, 10 3.5, 0 7" fill="#bbb" />
      </marker>
    </defs>
  </svg>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { usePlanDAGLayout } from '../../../features/chat/composables/usePlanDAGLayout';
import type { PlanStep } from '../../../features/chat/v2Types';
import PlanStepNode from './PlanStepNode.vue';

const props = defineProps<{
  steps: PlanStep[];
  width?: number;
}>();

const nodeWidth = 120;
const nodeHeight = 60;
const gapX = 40;
const gapY = 30;
const svgWidth = computed(() => props.width || 600);
const { layoutDAG } = usePlanDAGLayout();
const positions = computed(() => layoutDAG(props.steps, { width: svgWidth.value, nodeWidth, nodeHeight, gapX, gapY }));
const height = computed(() => {
  const max = Math.max(0, ...Array.from(positions.value.values()).map(p => p.y));
  return max + nodeHeight + 20;
});

const selectedId = ref<string | null>(null);

interface Edge { from: string; to: string; x1: number; y1: number; x2: number; y2: number }
const edges = computed<Edge[]>(() => {
  const out: Edge[] = [];
  for (const step of props.steps) {
    const toPos = positions.value.get(step.ID);
    if (!toPos) continue;
    for (const depId of step.DependsOn) {
      const fromPos = positions.value.get(depId);
      if (!fromPos) continue;
      out.push({
        from: depId, to: step.ID,
        x1: fromPos.x + nodeWidth / 2, y1: fromPos.y + nodeHeight,
        x2: toPos.x + nodeWidth / 2, y2: toPos.y,
      });
    }
  }
  return out;
});
</script>
```

- [ ] **Step 7: Create PlanStepDetailPanel.vue**

```vue
<!-- web/src/components/chat/v2/PlanStepDetailPanel.vue -->
<template>
  <div v-if="step" class="plan-step-detail">
    <div class="detail-header">
      <span class="detail-label">{{ step.Label }}</span>
      <q-badge :color="statusColor">{{ step.Status }}</q-badge>
    </div>
    <p v-if="step.Description">{{ step.Description }}</p>
    <div v-if="step.Result" class="detail-result">
      <h4>结果</h4>
      <pre>{{ step.Result.Output }}</pre>
    </div>
    <div v-if="step.Error" class="detail-error">
      <h4>错误</h4>
      <p>{{ step.Error.Message }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { PlanStep } from '../../../features/chat/v2Types';

const props = defineProps<{ step: PlanStep | null }>();
const statusColor = computed(() => ({
  pending: 'grey', running: 'blue', completed: 'green',
  failed: 'red', skipped: 'orange', partial_failure: 'red-7',
}[props.step?.Status || 'pending'] || 'grey'));
</script>
```

- [ ] **Step 8: Create PlanBoardCard.vue**

```vue
<!-- web/src/components/chat/v2/PlanBoardCard.vue -->
<template>
  <div class="plan-board-card">
    <div class="plan-header">
      <span>执行计划</span>
      <q-badge :color="statusColor">{{ planBoard.Status }}</q-badge>
    </div>
    <PlanDAG :steps="planBoard.Steps" />
    <PlanStepDetailPanel :step="selectedStep" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import type { PlanBoard } from '../../../features/chat/v2Types';
import PlanDAG from './PlanDAG.vue';
import PlanStepDetailPanel from './PlanStepDetailPanel.vue';

const props = defineProps<{ planBoard: PlanBoard }>();
const selectedStepId = ref<string | null>(null);
const selectedStep = computed(() =>
  props.planBoard.Steps.find(s => s.ID === selectedStepId.value) || null
);
const statusColor = computed(() => ({
  planning: 'orange', executing: 'blue', completed: 'green',
  failed: 'red', partial_failure: 'orange-8',
}[props.planBoard.Status] || 'grey'));
</script>
```

- [ ] **Step 9: Commit**

```bash
cd web && git add src/features/chat/composables/usePlanDAGLayout.ts src/features/chat/composables/__tests__/usePlanDAGLayout.spec.ts src/components/chat/v2/PlanStepNode.vue src/components/chat/v2/PlanDAG.vue src/components/chat/v2/PlanStepDetailPanel.vue src/components/chat/v2/PlanBoardCard.vue
git commit -m "feat(chat/v2): add plan DAG rendering (layout + SVG + detail panel)"
```

---

## Tier 4: Integration — Scroll, Blocked Status, Page Wiring

### Task 10: Rewrite useBlockedStatus for v2

**Files:**
- Modify: `web/src/features/chat/composables/useBlockedStatus.ts`
- Test: `web/src/features/chat/composables/__tests__/useBlockedStatus.spec.ts`

- [ ] **Step 1: Write failing test**

```typescript
// web/src/features/chat/composables/__tests__/useBlockedStatus.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { ref } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import { useBlockedStatus } from '../useBlockedStatus';
import type { Task, Step } from '../../v2Types';

describe('useBlockedStatus v2', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('returns not-blocked for completed task', () => {
    const store = useChatActivityStore();
    store.upsertTask({ ID: 'tk1', SessionID: 's1', UserMessage: '', Status: 'completed', Seq: 1, Version: 1, CreatedAt: '', UpdatedAt: '', CompletedAt: '' } as Task);
    const tasks = ref(store.getSessionTasks('s1'));
    const { blockedInfo } = useBlockedStatus(tasks);
    expect(blockedInfo.value.type).toBe('none');
  });

  it('detects tool_blocked step', () => {
    const store = useChatActivityStore();
    store.upsertTask({ ID: 'tk1', SessionID: 's1', UserMessage: '', Status: 'running', Seq: 1, Version: 1, CreatedAt: '', UpdatedAt: '', CompletedAt: null } as Task);
    store.upsertStep({ ID: 's1', TurnID: 't1', TaskID: 'tk1', SessionID: 's1', SpiritSessionID: 's1', Kind: 'action', AuthorAgentKey: 'a1', Seq: 1, Version: 1, Content: '', Reasoning: '', ToolName: 'shell', ToolCallID: '', ToolArgs: null, ToolResult: null, ToolDurationMs: 0, ToolErrorCode: '', Status: 'tool_blocked', IsFinal: false, StartedAt: '', CompletedAt: null } as Step);
    const tasks = ref(store.getSessionTasks('s1'));
    const { blockedInfo } = useBlockedStatus(tasks);
    expect(blockedInfo.value.type).toBe('tool');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/features/chat/composables/__tests__/useBlockedStatus.spec.ts`
Expected: FAIL — old implementation expects ActivityTreeNode[]

- [ ] **Step 3: Rewrite useBlockedStatus.ts**

```typescript
// web/src/features/chat/composables/useBlockedStatus.ts
import { computed, type Ref } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Task } from '../v2Types';

export type BlockedType = 'none' | 'tool' | 'confirm' | 'llm';

export interface BlockedInfo {
  type: BlockedType;
  agentKey?: string;
  stepId?: string;
}

export const EMPTY_BLOCKED: BlockedInfo = { type: 'none' };

/**
 * useBlockedStatus scans the store for steps with blocking statuses.
 * Replaces the old tree-walking implementation with direct store queries.
 */
export function useBlockedStatus(tasks: Ref<Task[]>) {
  const store = useChatActivityStore();

  const blockedInfo = computed<BlockedInfo>(() => {
    for (const task of tasks.value) {
      if (task.Status !== 'running') continue;
      // Scan all steps for this task's turns
      for (const turn of store.getTaskTurns(task.ID)) {
        for (const step of store.getTurnSteps(turn.ID)) {
          if (step.Status === 'tool_blocked') {
            return { type: 'tool', agentKey: step.AuthorAgentKey, stepId: step.ID };
          }
          if (step.Kind === 'confirm' && step.Status === 'running') {
            return { type: 'confirm', agentKey: step.AuthorAgentKey, stepId: step.ID };
          }
        }
      }
    }
    return EMPTY_BLOCKED;
  });

  return { blockedInfo };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/features/chat/composables/__tests__/useBlockedStatus.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd web && git add src/features/chat/composables/useBlockedStatus.ts src/features/chat/composables/__tests__/useBlockedStatus.spec.ts
git commit -m "feat(chat/v2): rewrite useBlockedStatus to query v2 store for blocking steps"
```

---

### Task 11: Adapt useChatMessageScroll + useScrollToActivity

**Files:**
- Modify: `web/src/features/chat/composables/useChatMessageScroll.ts`
- Modify: `web/src/features/chat/composables/useScrollToActivity.ts`
- Test: `web/src/features/chat/composables/__tests__/useChatMessageScroll.spec.ts`

- [ ] **Step 1: Modify useChatMessageScroll.ts**

The key change: replace `activityTree: ActivityTreeNode[]` with `tasks: Task[]`. The scroll logic (auto-scroll on new content, scroll-to-bottom button) stays the same.

Find the function signature and change:
```typescript
// Old:
export function useChatMessageScroll(opts: {
  sessionKey: Ref<string>;
  messages: Ref<Message[]>;
  messagesScrollEl: Ref<HTMLElement | null>;
  activityTree?: Ref<ActivityTreeNode[]>;
})

// New:
import type { Task } from '../v2Types';
export function useChatMessageScroll(opts: {
  sessionKey: Ref<string>;
  messages: Ref<Message[]>;
  messagesScrollEl: Ref<HTMLElement | null>;
  tasks?: Ref<Task[]>;
})
```

Update the watch on activityTree to watch `tasks` instead:
```typescript
// Old: watch(() => opts.activityTree?.value?.length, ...)
// New: watch(() => opts.tasks?.value?.length, ...)
```

- [ ] **Step 2: Modify useScrollToActivity.ts**

The existing module is only 26 lines. Update data attributes:
```typescript
// Old: data-agent-key / data-team-stage-id
// New: same data attributes (v2 components use them too)
```

No structural change needed — the locate command pattern stays the same.

- [ ] **Step 3: Write a basic scroll test**

```typescript
// web/src/features/chat/composables/__tests__/useChatMessageScroll.spec.ts
import { describe, it, expect } from 'vitest';
import { ref } from 'vue';
import { useChatMessageScroll } from '../useChatMessageScroll';

describe('useChatMessageScroll v2', () => {
  it('accepts tasks ref instead of activityTree', () => {
    const { showScrollBtn, scrollToBottom } = useChatMessageScroll({
      sessionKey: ref('s1'),
      messages: ref([]),
      messagesScrollEl: ref(null),
      tasks: ref([]),
    });
    expect(showScrollBtn.value).toBe(false);
    expect(typeof scrollToBottom).toBe('function');
  });
});
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/features/chat/composables/__tests__/useChatMessageScroll.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd web && git add src/features/chat/composables/useChatMessageScroll.ts src/features/chat/composables/useScrollToActivity.ts src/features/chat/composables/__tests__/useChatMessageScroll.spec.ts
git commit -m "feat(chat/v2): adapt useChatMessageScroll and useScrollToActivity to v2 Task[] model"
```

---

### Task 12: Wire v2 Pipeline into ChatPage + useChatWorkspace

**Files:**
- Modify: `web/src/features/chat/composables/useChatWorkspace.ts`
- Modify: `web/src/pages/ChatPage.vue`
- Modify: `web/src/components/chat/ChatMessagePanel.vue`
- Modify: `web/src/components/chat/ChatMessageList.vue`

This task connects the new store + router + components into the existing page structure. The key integration points:
1. `useChatWorkspace` instantiates the v2 store + event router
2. WS transport's `onV2Event` feeds into the event router
3. `ChatPage` renders `<SessionPanel>` instead of `<ActivityStream>`
4. `useBlockedStatus` receives `tasks` ref instead of `activityTree`

- [ ] **Step 1: Modify useChatWorkspace.ts**

Find the section where `useActivityTimeline` is instantiated (around line 117) and replace:

```typescript
// OLD:
import { useActivityTimeline } from './useActivityTimeline';
// ...
const activityTimeline = useActivityTimeline();

// NEW:
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import { useChatEventRouter } from './useChatEventRouter';
// ...
const activityStore = useChatActivityStore();
const eventRouter = useChatEventRouter(activityStore);
```

Update the `handleActivityEvent` wrapper to also handle v2 events:
```typescript
// Add a new handler for v2 events:
const handleV2Event = (envelope: V2WsEnvelope) => {
  eventRouter.dispatch(envelope);
};

// In the stream manager + global hub consumer setup, add onV2Event:
// (find where onActivityEvent is passed and add onV2Event alongside)
```

Expose the store and tasks for the page:
```typescript
return {
  // ... existing returns ...
  activityStore,
  v2Tasks: computed(() => activityStore.getSessionTasks(sessionId.value)),
};
```

- [ ] **Step 2: Modify ChatPage.vue**

Replace ActivityStream with SessionPanel:

```vue
<!-- OLD: -->
<ActivityStream :activity-tree="session.activityTimeline.activityTree" ... />

<!-- NEW: -->
<SessionPanelV2 :session-id="sessionId" />
```

Update the import and blocked status:
```typescript
import SessionPanelV2 from '../components/chat/v2/SessionPanel.vue';
import { useChatActivityStore } from '../stores/chat/activityV2Store';

const activityStore = useChatActivityStore();
const tasks = computed(() => activityStore.getSessionTasks(sessionId));
const { blockedInfo } = useBlockedStatus(tasks);
```

- [ ] **Step 3: Modify ChatMessagePanel.vue + ChatMessageList.vue**

Replace `activityTree` prop with `sessionId` prop and render `<SessionPanelV2>`.

- [ ] **Step 4: Run lint + type-check**

Run: `cd web && pnpm lint`
Expected: No errors (or only pre-existing warnings)

- [ ] **Step 5: Commit**

```bash
cd web && git add src/features/chat/composables/useChatWorkspace.ts src/pages/ChatPage.vue src/components/chat/ChatMessagePanel.vue src/components/chat/ChatMessageList.vue
git commit -m "feat(chat/v2): wire v2 store + SessionPanel into ChatPage pipeline"
```

---

## Tier 5: Cleanup — Delete Old Files + Backend Compat Adapter

### Task 13: Delete Old Frontend Files

**Files:**
- Delete: `web/src/features/chat/composables/useActivityTimeline.ts`
- Delete: `web/src/features/chat/activityTypes.ts`
- Delete: `web/src/features/chat/streamEventTypes.ts`
- Delete: `web/src/realtime/activityEvent.ts`
- Delete: `web/src/components/chat/ActivityStream.vue`
- Delete: `web/src/components/chat/PlanBlock.vue`
- Delete: `web/src/components/chat/TeamCard.vue`
- Delete: `web/src/components/chat/AgentCard.vue`
- Delete: associated test files (useActivityTimeline.spec.ts, etc.)

- [ ] **Step 1: Search for remaining imports of deleted files**

Run: `cd web && grep -r "useActivityTimeline\|activityTypes\|streamEventTypes\|realtime/activityEvent\|ActivityStream\.vue\|PlanBlock\.vue\|TeamCard\.vue\|AgentCard\.vue" src/ --include="*.ts" --include="*.vue" -l`

Fix any remaining imports (they should already be replaced by Task 12).

- [ ] **Step 2: Delete files**

```bash
cd web
rm src/features/chat/composables/useActivityTimeline.ts
rm src/features/chat/activityTypes.ts
rm src/features/chat/streamEventTypes.ts
rm src/realtime/activityEvent.ts
rm src/components/chat/ActivityStream.vue
rm src/components/chat/PlanBlock.vue
rm src/components/chat/TeamCard.vue
rm src/components/chat/AgentCard.vue
rm -f src/features/chat/composables/__tests__/useActivityTimeline.spec.ts
```

- [ ] **Step 3: Run build to verify no broken imports**

Run: `cd web && pnpm build`
Expected: BUILD SUCCESS (or fix remaining references)

- [ ] **Step 4: Run full test suite**

Run: `cd web && pnpm test`
Expected: All tests PASS (excluding deleted test files)

- [ ] **Step 5: Commit**

```bash
cd web && git add -A
git commit -m "refactor(chat/v2): delete legacy ActivityEvent frontend (8 files, ~3500 lines)"
```

---

### Task 14: Delete Backend Compat Adapter

**Files:**
- Delete: `internal/agent/v2/compat_adapter.go`
- Delete: `internal/agent/v2/compat_adapter_test.go`
- Modify: `cmd/admin/wire.go` (remove CompatAdapter provider if present)
- Modify: `internal/agent/v2/sequencer.go` (remove CompatAdapter.PublishV1 call if present)

- [ ] **Step 1: Search for CompatAdapter usage**

Run: `grep -r "CompatAdapter\|provideCompatAdapter\|PublishV1" internal/ cmd/ --include="*.go" -l`

- [ ] **Step 2: Remove CompatAdapter wiring from sequencer.go**

If `sequencer.go` calls `a.compat.PublishV1(ctx, e)`, remove that line and the `compat` field from the Sequencer struct.

- [ ] **Step 3: Remove CompatAdapter from wire.go**

If `wire.go` has `provideCompatAdapter` or binds `*CompatAdapter`, remove those lines.

- [ ] **Step 4: Delete compat_adapter files**

```bash
rm internal/agent/v2/compat_adapter.go
rm internal/agent/v2/compat_adapter_test.go
```

- [ ] **Step 5: Build + test**

Run: `go build -tags=pgvector ./... && go test -tags=pgvector ./internal/agent/v2/... -count=1`
Expected: BUILD + TESTS PASS

- [ ] **Step 6: Commit**

```bash
git add internal/agent/v2/compat_adapter.go internal/agent/v2/compat_adapter_test.go cmd/admin/wire.go internal/agent/v2/sequencer.go
git commit -m "refactor(agent/v2): remove CompatAdapter (Phase 2 frontend rewrite complete)"
```

---

### Task 15: Full Verification

- [ ] **Step 1: Backend full check**

Run: `make api && make wire && make build && make test && make lint`
Expected: All PASS

- [ ] **Step 2: Frontend full check**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: All PASS

- [ ] **Step 3: E2E smoke test**

Start the admin server with `configs/smoke_postgres.yaml`, send a chat message, verify:
1. Chat completes (HTTP 200)
2. v2 tables populated (tasks_v2, turns_v2, steps_v2)
3. Frontend UI renders the new components (if running dev server)
4. WS v2 events received by frontend

- [ ] **Step 4: Final commit (if any lint fixes)**

```bash
git add -A
git commit -m "chore: Phase 2 verification complete"
```

---

## Self-Review Notes

**Spec coverage check:**
- ✅ §3.6.1 Store data structure → Task 2 (activityV2Store)
- ✅ §3.6.2 Render tree construction → Task 5 (useTaskTree)
- ✅ §3.6.3 Component hierarchy → Tasks 7, 8, 9
- ✅ §3.6.4 WS event handling → Tasks 3, 4
- ✅ §3.6.5 Scroll & locate → Task 11
- ✅ §3.7 Plan DAG → Task 9
- ✅ §9.3 Frontend new/modify files → All tasks
- ✅ §9.4 Frontend delete files → Task 13
- ✅ Compat adapter removal → Task 14

**Known gaps:**
- `TaskCancelledEvent` struct doesn't exist in backend (only constant) — frontend ignores this kind safely via default case
- NoticeBlock/ConfirmBlock/ErrorBlock are reused as-is from existing components — they may need prop adaptation in a follow-up if they depend on `streamEventTypes`
- `activityPresentation.ts` (builtinLabels) and `activityTimelineTypes.ts` (re-export barrel) are not explicitly deleted — they become dead code and should be cleaned up if lint flags them
