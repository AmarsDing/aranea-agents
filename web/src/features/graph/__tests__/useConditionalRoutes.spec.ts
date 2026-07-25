// web/src/features/graph/__tests__/useConditionalRoutes.spec.ts
// 条件路由编辑经 undoRedo 命令栈的回归测试：
// 每个操作必须「恰好应用一次」，undo 精确还原且不污染兄弟 CE。
import { describe, it, expect, vi } from 'vitest';
import { reactive, computed } from 'vue';
import { useConditionalRoutes } from '../useConditionalRoutes';
import { useGraphUndoRedo } from '../useGraphUndoRedo';
import type { GraphDefinition, ConditionalEdgeDef } from '../types';

function makeGraphDef(): GraphDefinition {
  return reactive<GraphDefinition>({
    id: 'g1',
    name: 'G',
    description: '',
    stateFields: [],
    nodes: [],
    edges: [],
    conditionalEdges: [],
    subgraphs: [],
    entryPoint: '',
    finishPoint: '',
    enableCheckpoint: true,
    executionEngine: 'bsp',
    interruptBefore: [],
    interruptAfter: [],
    metadata: {},
    version: 0,
    sortOrder: 0,
    createdAt: '',
    updatedAt: '',
  }) as GraphDefinition;
}

function setup(selectedNodeId = 'r1') {
  const graphDef = makeGraphDef();
  const markDirty = vi.fn();
  const ur = useGraphUndoRedo(graphDef, markDirty);
  const routes = useConditionalRoutes(
    computed(() => graphDef),
    computed(() => selectedNodeId),
    computed(() => ur),
    vi.fn(),
    computed(() => [
      { label: 'a', value: 'a' },
      { label: 'b', value: 'b' },
    ]),
  );
  return { graphDef, ur, routes };
}

describe('useConditionalRoutes - undoRedo 命令恰好应用一次', () => {
  it('addConditionalEdge 恰好添加一条 CE；undo 全量移除', () => {
    const { graphDef, ur, routes } = setup();

    routes.addConditionalEdge();
    const matches = graphDef.conditionalEdges.filter((ce) => ce.from === 'r1');
    expect(matches).toHaveLength(1);
    expect(matches[0].pathMap).toEqual({ default: 'a' });

    ur.undo();
    expect(graphDef.conditionalEdges.filter((ce) => ce.from === 'r1')).toHaveLength(0);
  });

  it('removePathMapEntry 删空 CE：恰好移除该 CE，兄弟 CE 不受影响；undo 精确还原', () => {
    const { graphDef, ur, routes } = setup();
    const ce0: ConditionalEdgeDef = { from: 'r1', condFuncRef: 'fn1', pathMap: { ok: 'a' } };
    const ce1: ConditionalEdgeDef = { from: 'r2', condFuncRef: 'fn2', pathMap: { default: 'b' } };
    graphDef.conditionalEdges.push(ce0, ce1);

    // localIdx=0 → ce0（from=r1），删除其唯一 label → 整条移除
    routes.removePathMapEntry(0, 'ok');
    expect(graphDef.conditionalEdges).toHaveLength(1);
    expect(graphDef.conditionalEdges[0].from).toBe('r2');
    // 兄弟 CE 完好
    expect(graphDef.conditionalEdges[0].pathMap).toEqual({ default: 'b' });

    ur.undo();
    expect(graphDef.conditionalEdges).toHaveLength(2);
    expect(graphDef.conditionalEdges[0].from).toBe('r1');
    expect(graphDef.conditionalEdges[0].pathMap).toEqual({ ok: 'a' });
    expect(graphDef.conditionalEdges[1].pathMap).toEqual({ default: 'b' });
  });

  it('updatePathMapLabel/undo 精确还原 label 重命名', () => {
    const { graphDef, ur, routes } = setup();
    const ce: ConditionalEdgeDef = { from: 'r1', condFuncRef: '', pathMap: { ok: 'a', fail: 'b' } };
    graphDef.conditionalEdges.push(ce);

    routes.updatePathMapLabel(0, 'ok', 'success');
    expect(graphDef.conditionalEdges[0].pathMap).toEqual({ fail: 'b', success: 'a' });

    ur.undo();
    expect(graphDef.conditionalEdges[0].pathMap).toEqual({ ok: 'a', fail: 'b' });
  });
});
