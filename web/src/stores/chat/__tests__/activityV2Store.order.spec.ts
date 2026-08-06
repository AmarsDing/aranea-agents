import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useChatActivityStore } from '../activityV2Store';
import type { PlanStep, GraphNode } from '../../../features/chat/v2Types';

// 2026-08-06 顺序修复（20:45 会话看板/流程图乱序根因）：
// 1. PlanStep.StartedAt 在 dispatch 时被覆盖为实际开始时间，time 主排序导致
//    已运行 step 排到 pending step 之后 → 必须以 Seq 主排序。
// 2. GraphNode 无 Seq 字段，getGraphStageNodes 依赖 Map 迭代顺序（事件到达
//    顺序）→ 必须经 DagNodeID 派生 PlanStep.Seq 排序。

function mkPlanStep(over: Partial<PlanStep> = {}): PlanStep {
  return {
    ID: 's1',
    PlanID: 'pb1',
    TaskID: 't1',
    Label: 'step',
    Description: '',
    DependsOn: [],
    MappedTeamStageID: '',
    Status: 'pending',
    AutoSynthesis: false,
    StartedAt: '',
    CompletedAt: null,
    Seq: 1,
    Version: 1,
    Result: null,
    Error: null,
    AgentKeys: [],
    Deliverables: [],
    InputContract: [],
    ...over,
  };
}

function mkGraphNode(over: Partial<GraphNode> = {}): GraphNode {
  return {
    ID: 'gn1',
    GraphStageID: 'gs1',
    Label: 'node',
    DagNodeID: '',
    TeamStageID: '',
    Status: 'pending',
    DependsOn: [],
    ...over,
  };
}

describe('activityV2Store ordering', () => {
  let store: ReturnType<typeof useChatActivityStore>;

  beforeEach(() => {
    setActivePinia(createPinia());
    store = useChatActivityStore();
  });

  it('getPlanBoardSteps sorts by Seq even when StartedAt is overwritten at dispatch', () => {
    // 复现乱序：s1 已开始运行（StartedAt 被覆盖为较晚时间），s2/s3 仍 pending
    // （保留创建时的较早时间）。time 主排序会把 s1 排到最后。
    const created = '2026-08-06T20:45:00Z';
    const started = '2026-08-06T20:45:05Z';
    store.upsertPlanStep(mkPlanStep({ ID: 's2', Seq: 2, StartedAt: created }));
    store.upsertPlanStep(mkPlanStep({ ID: 's3', Seq: 3, StartedAt: created }));
    store.upsertPlanStep(mkPlanStep({ ID: 's1', Seq: 1, Status: 'running', StartedAt: started }));

    const ids = store.getPlanBoardSteps('pb1').map((s) => s.ID);
    expect(ids).toEqual(['s1', 's2', 's3']);
  });

  it('getPlanBoardSteps falls back to time when either side lacks Seq', () => {
    // 遗留数据 Seq=0：保持原有 time 排序行为。
    const t1 = '2026-08-06T20:45:00Z';
    const t2 = '2026-08-06T20:46:00Z';
    store.upsertPlanStep(mkPlanStep({ ID: 'b', Seq: 0, StartedAt: t2 }));
    store.upsertPlanStep(mkPlanStep({ ID: 'a', Seq: 0, StartedAt: t1 }));

    const ids = store.getPlanBoardSteps('pb1').map((s) => s.ID);
    expect(ids).toEqual(['a', 'b']);
  });

  it('getGraphStageNodes derives order from PlanStep.Seq via DagNodeID', () => {
    store.upsertPlanStep(mkPlanStep({ ID: 's1', Seq: 1 }));
    store.upsertPlanStep(mkPlanStep({ ID: 's2', Seq: 2 }));
    store.upsertPlanStep(mkPlanStep({ ID: 's3', Seq: 3 }));
    // 以乱序到达（懒加载 / 事件重放）。
    store.upsertGraphNode(mkGraphNode({ ID: 's3', DagNodeID: 's3', Label: 'c' }));
    store.upsertGraphNode(mkGraphNode({ ID: 's1', DagNodeID: 's1', Label: 'a' }));
    store.upsertGraphNode(mkGraphNode({ ID: 's2', DagNodeID: 's2', Label: 'b' }));

    const ids = store.getGraphStageNodes('gs1').map((n) => n.ID);
    expect(ids).toEqual(['s1', 's2', 's3']);
  });

  it('getGraphStageNodes keeps unmatched nodes last in stable insertion order', () => {
    store.upsertPlanStep(mkPlanStep({ ID: 's1', Seq: 1 }));
    store.upsertGraphNode(mkGraphNode({ ID: 'x1', DagNodeID: 'missing', Label: 'x1' }));
    store.upsertGraphNode(mkGraphNode({ ID: 's1', DagNodeID: 's1', Label: 'a' }));
    store.upsertGraphNode(mkGraphNode({ ID: 'x2', DagNodeID: '', Label: 'x2' }));

    const ids = store.getGraphStageNodes('gs1').map((n) => n.ID);
    expect(ids).toEqual(['s1', 'x1', 'x2']);
  });
});
