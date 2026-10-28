import { beforeEach, describe, expect, it } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { ref } from 'vue';
import { useObserveGraph, useObserveNodeEnrichment } from '../useObserveGraph';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import type { GraphNode, GraphStage, PlanStep } from '../../v2Types';

function node(id: string, depends: string[] = [], stage = 'G'): GraphNode {
  return {
    ID: id,
    GraphStageID: stage,
    Label: id,
    DagNodeID: `plan:${id}`,
    TeamStageID: '',
    Status: 'completed',
    DependsOn: depends,
  };
}
function stage(id: string, nodes: GraphNode[], seq = 1): GraphStage {
  return {
    ID: id,
    TaskID: 'T',
    TurnID: 'turn',
    SessionID: 'S',
    PlanBoardID: 'P',
    Nodes: nodes,
    Status: 'completed',
    StartedAt: '',
    CompletedAt: null,
    Seq: seq,
    Version: 1,
  };
}
function step(): PlanStep {
  return {
    ID: 'plan:A',
    PlanID: 'P',
    TaskID: 'T',
    Label: 'A',
    Description: 'Actual subtask',
    DependsOn: [],
    MappedTeamStageID: '',
    Status: 'completed',
    AutoSynthesis: false,
    StartedAt: '',
    CompletedAt: null,
    Seq: 1,
    Version: 1,
    Error: null,
    Result: {
      Output: 'Actual persisted result',
      MemberReports: [],
      TokensUsed: { PromptTokens: 0, CompletionTokens: 0, TotalTokens: 0 },
      DurationMs: 0,
    },
  };
}

beforeEach(() => setActivePinia(createPinia()));

describe('observation graph presentation', () => {
  it('reads task description and result from their real PlanStep owner', () => {
    const store = useChatActivityStore();
    store.planSteps.set('plan:A', step());
    const view = useObserveNodeEnrichment();
    expect(view.extractDescription(node('A'))).toBe('Actual subtask');
    expect(view.extractTextOutput(node('A'))).toBe('Actual persisted result');
    expect(view.extractTextOutput({ ...node('A'), Status: 'running' })).toBeUndefined();
  });

  it('highlights real edges across multiple dependency hops and both diamond branches', () => {
    const store = useChatActivityStore();
    store.upsertGraphStage(
      stage('G', [node('A'), node('B', ['A']), node('C', ['A']), node('D', ['B', 'C']), node('E', ['D'])]),
    );
    const view = useObserveGraph(ref('S'));
    const chain = view.getDependencyChain('D');
    expect([...chain.nodeIds].sort()).toEqual(['A', 'B', 'C', 'D', 'E']);
    expect([...chain.edgeIds].sort()).toEqual(['A→B', 'A→C', 'B→D', 'C→D', 'D→E']);
    expect(chain.edgeIds.has('A→D')).toBe(false);
  });

  it('uses the latest stage by default and respects an explicitly selected stage', () => {
    const store = useChatActivityStore();
    const old = stage('old', [node('old-node', [], 'old')], 1);
    const latest = stage('latest', [node('new-node', [], 'latest')], 2);
    store.upsertGraphStage(old);
    store.upsertGraphStage(latest);
    expect(useObserveGraph(ref('S')).graphStage.value?.ID).toBe('latest');
    const selected = ref<GraphStage | null>(old);
    const view = useObserveGraph(ref('S'), selected);
    expect(view.nodes.value.map((row) => row.ID)).toEqual(['old-node']);
    selected.value = latest;
    expect(view.nodes.value.map((row) => row.ID)).toEqual(['new-node']);
    selected.value = null;
    expect(view.nodes.value).toEqual([]);
    selected.value = { ...latest, SessionID: 'other-session' };
    expect(view.nodes.value).toEqual([]);
  });

  it('terminates dependency traversal on malformed cycles without inventing edges', () => {
    useChatActivityStore().upsertGraphStage(stage('G', [node('A', ['B']), node('B', ['A'])]));
    const chain = useObserveGraph(ref('S')).getDependencyChain('A');
    expect([...chain.edgeIds].sort()).toEqual(['A→B', 'B→A']);
  });
});
