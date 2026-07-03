// web/src/features/chat/composables/__tests__/useTaskTree.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
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
    store.upsertTask({
      ID: 'tk1',
      SessionID: 's1',
      UserMessage: 'hi',
      Status: 'completed',
      Seq: 1,
      Version: 1,
      CreatedAt: '',
      UpdatedAt: '',
      CompletedAt: null,
    } as Task);
    store.upsertTurn({
      ID: 'turn2',
      TaskID: 'tk1',
      SessionID: 's1',
      SpiritSessionID: 's1',
      ParentTurnID: '',
      AgentKey: 'a1',
      TeamID: '',
      TeamStageID: '',
      Seq: 2,
      Version: 1,
      Status: 'completed',
      StartedAt: '',
      CompletedAt: null,
    } as never);
    store.upsertTurn({
      ID: 'turn1',
      TaskID: 'tk1',
      SessionID: 's1',
      SpiritSessionID: 's1',
      ParentTurnID: '',
      AgentKey: 'a1',
      TeamID: '',
      TeamStageID: '',
      Seq: 1,
      Version: 1,
      Status: 'completed',
      StartedAt: '',
      CompletedAt: null,
    } as never);
    store.upsertStep({
      ID: 's2',
      TurnID: 'turn1',
      TaskID: 'tk1',
      SessionID: 's1',
      SpiritSessionID: 's1',
      Kind: 'reply',
      AuthorAgentKey: 'a1',
      Seq: 2,
      Version: 1,
      Content: 'world',
      Reasoning: '',
      ToolName: '',
      ToolCallID: '',
      ToolArgs: null,
      ToolResult: null,
      ToolDurationMs: 0,
      ToolErrorCode: '',
      Status: 'completed',
      IsFinal: true,
      StartedAt: '',
      CompletedAt: null,
    } as never);
    store.upsertStep({
      ID: 's1',
      TurnID: 'turn1',
      TaskID: 'tk1',
      SessionID: 's1',
      SpiritSessionID: 's1',
      Kind: 'thinking',
      AuthorAgentKey: 'a1',
      Seq: 1,
      Version: 1,
      Content: '',
      Reasoning: 'think',
      ToolName: '',
      ToolCallID: '',
      ToolArgs: null,
      ToolResult: null,
      ToolDurationMs: 0,
      ToolErrorCode: '',
      Status: 'completed',
      IsFinal: false,
      StartedAt: '',
      CompletedAt: null,
    } as never);

    const { buildTaskTree } = useTaskTree(store);
    const tree = buildTaskTree('tk1');
    expect(tree?.task.ID).toBe('tk1');
    expect(tree?.turnTrees.map((t) => t.turn.ID)).toEqual(['turn1', 'turn2']);
    expect(tree?.turnTrees[0].steps.map((s) => s.ID)).toEqual(['s1', 's2']);
  });

  it('includes team stages and plan boards for task', () => {
    store.upsertTask({
      ID: 'tk1',
      SessionID: 's1',
      UserMessage: 'hi',
      Status: 'running',
      Seq: 1,
      Version: 1,
      CreatedAt: '',
      UpdatedAt: '',
      CompletedAt: null,
    } as Task);
    store.upsertTeamStage({
      ID: 'ts1',
      TaskID: 'tk1',
      TurnID: '',
      SessionID: 's1',
      TeamID: 'team1',
      DagNodeID: '',
      DependsOn: [],
      Status: 'running',
      Stage: 'executing',
      Members: [],
      Strategy: 'parallel',
      StartedAt: '',
      CompletedAt: null,
      Seq: 1,
      Version: 1,
    } as never);
    store.upsertPlanBoard({
      ID: 'pb1',
      TaskID: 'tk1',
      TurnID: '',
      SessionID: 's1',
      Strategy: 'dag',
      Status: 'executing',
      Steps: [],
      StartedAt: '',
      CompletedAt: null,
      Seq: 1,
      Version: 1,
    } as never);

    const { buildTaskTree } = useTaskTree(store);
    const tree = buildTaskTree('tk1');
    expect(tree?.teamStages.map((ts) => ts.ID)).toEqual(['ts1']);
    expect(tree?.planBoards.map((pb) => pb.ID)).toEqual(['pb1']);
  });
});
