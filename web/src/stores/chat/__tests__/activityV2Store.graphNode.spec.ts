/**
 * GraphNode 终态粘性守卫契约（D9-21）：live/snapshot 双源生命周期秩仲裁。
 *  rank(pending)=0 < rank(running)=1 < rank(completed|failed|interrupted)=2
 *  ① live pending→running→completed 全部生效
 *  ② snapshot completed→running：Status 保持 completed，元数据照常合并
 *  ③ snapshot running→pending：Status 回退被拒
 *  ④ snapshot pending→running / running→completed 前向放行
 *  ⑤ live resume failed→pending→running 放行
 *  ⑥ live completed→running 被汇点防御拒
 *  ⑦ upsertGraphStage 内嵌 Nodes 同 source 透传（snapshot 走守卫、live 放行）
 *  ⑧ 同秩终态调整（failed→interrupted）经 snapshot 允许
 */
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useChatActivityStore } from '../activityV2Store';
import type { GraphNode, GraphStage } from '../../../features/chat/v2Types';

function mkGraphNode(over: Partial<GraphNode> = {}): GraphNode {
  return {
    ID: 'n-1',
    GraphStageID: 'gs-1',
    Label: 'node',
    DagNodeID: 'dag-1',
    TeamStageID: '',
    Status: 'pending',
    DependsOn: [],
    ...over,
  };
}

function mkGraphStage(over: Partial<GraphStage> = {}): GraphStage {
  return {
    ID: 'gs-1',
    TaskID: 't-1',
    TurnID: 'tu-1',
    SessionID: 's-1',
    PlanBoardID: 'pb-1',
    Nodes: [],
    Status: 'running',
    StartedAt: '',
    CompletedAt: null,
    Seq: 1,
    Version: 1,
    ...over,
  };
}

describe('activityV2Store graph node terminal-stickiness guard (D9-21)', () => {
  let store: ReturnType<typeof useChatActivityStore>;

  beforeEach(() => {
    setActivePinia(createPinia());
    store = useChatActivityStore();
  });

  it('① live pending→running→completed 全部生效', () => {
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'pending' }));
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'running' }));
    expect(store.graphNodes.get('n-1')?.Status).toBe('running');
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'completed' }));
    expect(store.graphNodes.get('n-1')?.Status).toBe('completed');
  });

  it('② snapshot completed→running：Status 保持 completed，TeamStageID/Label 合并更新', () => {
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'completed', TeamStageID: 'ts-old', Label: 'old' }), 'live');
    store.upsertGraphNode(
      mkGraphNode({ ID: 'n-1', Status: 'running', TeamStageID: 'ts-new', Label: 'new' }),
      'snapshot',
    );
    const gn = store.graphNodes.get('n-1');
    expect(gn?.Status).toBe('completed');
    expect(gn?.TeamStageID).toBe('ts-new');
    expect(gn?.Label).toBe('new');
  });

  it('③ snapshot running→pending 被拒（rank 单调不降）', () => {
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'running' }), 'live');
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'pending' }), 'snapshot');
    expect(store.graphNodes.get('n-1')?.Status).toBe('running');
  });

  it('④ snapshot pending→running、running→completed 前向放行', () => {
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'pending' }), 'live');
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'running' }), 'snapshot');
    expect(store.graphNodes.get('n-1')?.Status).toBe('running');
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'completed' }), 'snapshot');
    expect(store.graphNodes.get('n-1')?.Status).toBe('completed');
  });

  it('⑤ live resume failed→pending→running 放行（后端 machines 合法转移）', () => {
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'failed' }), 'live');
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'pending' }), 'live');
    expect(store.graphNodes.get('n-1')?.Status).toBe('pending');
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'running' }), 'live');
    expect(store.graphNodes.get('n-1')?.Status).toBe('running');
  });

  it('⑥ live completed→running 被 completed 汇点防御拒', () => {
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'completed' }), 'live');
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'running' }), 'live');
    expect(store.graphNodes.get('n-1')?.Status).toBe('completed');
  });

  it('⑦ upsertGraphStage 内嵌 Nodes 按同一 source 透传粘性守卫', () => {
    // snapshot：内嵌节点走 rank 守卫（completed 不得回退）
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', GraphStageID: 'gs-1', Status: 'completed' }), 'live');
    store.upsertGraphStage(
      mkGraphStage({ ID: 'gs-1', Nodes: [mkGraphNode({ ID: 'n-1', GraphStageID: 'gs-1', Status: 'running' })] }),
      'snapshot',
    );
    expect(store.graphNodes.get('n-1')?.Status).toBe('completed');

    // live（router 路径）：合法转移放行（failed→pending resume）
    store.upsertGraphNode(mkGraphNode({ ID: 'n-2', GraphStageID: 'gs-1', Status: 'failed' }), 'live');
    store.upsertGraphStage(
      mkGraphStage({
        ID: 'gs-1',
        Version: 2,
        Nodes: [mkGraphNode({ ID: 'n-2', GraphStageID: 'gs-1', Status: 'pending' })],
      }),
      'live',
    );
    expect(store.graphNodes.get('n-2')?.Status).toBe('pending');
  });

  it('⑧ 同秩终态调整（failed→interrupted）经 snapshot 允许', () => {
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'failed' }), 'live');
    store.upsertGraphNode(mkGraphNode({ ID: 'n-1', Status: 'interrupted' }), 'snapshot');
    expect(store.graphNodes.get('n-1')?.Status).toBe('interrupted');
  });
});
