// web/src/features/graph/__tests__/useGraphUndoRedo.spec.ts
// 撤销/重做命令栈契约测试：
// 核心契约 —— 调用方不得预改 graphDef；execute() 通过 redo() 完成首次应用。
// 每个 push* 必须「恰好应用一次」，undo 必须精确还原（不污染相邻实体）。
import { describe, it, expect, vi } from 'vitest';
import { reactive } from 'vue';
import { useGraphUndoRedo } from '../useGraphUndoRedo';
import type { GraphDefinition, NodeDef, ConditionalEdgeDef, StateFieldDef } from '../types';
import { readGraphLayout } from '../editor/graphLayout';

function makeNode(id: string): NodeDef {
  return {
    id,
    funcRef: '',
    interruptBefore: false,
    interruptAfter: false,
    type: 'function',
    description: '',
    instruction: '',
    modelName: '',
    toolNames: [],
    agentName: '',
    destinations: [],
    requiredRole: '',
    assignmentMode: 'static',
    assignmentStrategy: '',
    reviewerAgent: '',
    reviewRules: '',
    timeoutSeconds: 0,
    heartbeatIntervalSeconds: 0,
    enableLeaseExtension: false,
    retryMaxAttempts: 0,
    failureAction: '',
    fallbackAgent: '',
    inputMapperJson: '',
    outputMapperJson: '',
    isolatedMessages: false,
    inputFromLastResponse: false,
    cacheEnabled: false,
    cacheTtlSeconds: 0,
  };
}

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

function setup() {
  const graphDef = makeGraphDef();
  const markDirty = vi.fn();
  const ur = useGraphUndoRedo(graphDef, markDirty);
  return { graphDef, markDirty, ur };
}

describe('useGraphUndoRedo - 命令恰好应用一次（execute 即首次应用）', () => {
  it('pushAddNode 恰好添加一个节点；undo 删除；redo 恢复', () => {
    const { graphDef, ur } = setup();
    ur.pushAddNode(makeNode('n1'), 0);
    expect(graphDef.nodes).toHaveLength(1);
    expect(graphDef.nodes[0].id).toBe('n1');

    ur.undo();
    expect(graphDef.nodes).toHaveLength(0);

    ur.redo();
    expect(graphDef.nodes).toHaveLength(1);
    expect(graphDef.nodes[0].id).toBe('n1');
  });

  it('pushDuplicateNode 恰好添加一个副本；undo 全量移除', () => {
    const { graphDef, ur } = setup();
    graphDef.nodes.push(makeNode('src'));
    ur.pushDuplicateNode('src', makeNode('src_copy'), 1);
    expect(graphDef.nodes).toHaveLength(2);
    expect(graphDef.nodes.filter((n) => n.id === 'src_copy')).toHaveLength(1);

    ur.undo();
    expect(graphDef.nodes.filter((n) => n.id === 'src_copy')).toHaveLength(0);
    expect(graphDef.nodes).toHaveLength(1);
  });

  it('pushAddEdge 恰好添加一条边；undo 后图中无该边', () => {
    const { graphDef, ur } = setup();
    ur.pushAddEdge({ from: 'a', to: 'b', kind: '' });
    expect(graphDef.edges).toHaveLength(1);

    ur.undo();
    expect(graphDef.edges).toHaveLength(0);

    ur.redo();
    expect(graphDef.edges).toHaveLength(1);
  });

  it('pushAddStateField 恰好添加一个字段；undo 全量移除', () => {
    const { graphDef, ur } = setup();
    const field: StateFieldDef = {
      name: 'f1',
      type: 'string',
      reducer: 'cover',
      required: false,
      disableDeepCopy: false,
    };
    ur.pushAddStateField(field, 0);
    expect(graphDef.stateFields).toHaveLength(1);

    ur.undo();
    expect(graphDef.stateFields).toHaveLength(0);
  });

  it('pushRemoveStateField 只删除目标字段，undo 还原', () => {
    const { graphDef, ur } = setup();
    const a: StateFieldDef = { name: 'a', type: 'string', reducer: 'cover', required: false, disableDeepCopy: false };
    const b: StateFieldDef = { name: 'b', type: 'number', reducer: 'cover', required: false, disableDeepCopy: false };
    graphDef.stateFields.push(a, b);

    ur.pushRemoveStateField(a, 0);
    expect(graphDef.stateFields.map((f) => f.name)).toEqual(['b']);

    ur.undo();
    expect(graphDef.stateFields.map((f) => f.name)).toEqual(['a', 'b']);
  });

  it('pushMoveNodes undo/redo 正确写回布局位置', () => {
    const { graphDef, ur } = setup();
    graphDef.nodes.push(makeNode('n1'));
    ur.pushMoveNodes([{ nodeId: 'n1', oldPos: { x: 10, y: 20 }, newPos: { x: 100, y: 200 } }]);
    expect(readGraphLayout(graphDef)['n1']).toEqual({ x: 100, y: 200 });

    ur.undo();
    expect(readGraphLayout(graphDef)['n1']).toEqual({ x: 10, y: 20 });

    ur.redo();
    expect(readGraphLayout(graphDef)['n1']).toEqual({ x: 100, y: 200 });
  });
});

describe('useGraphUndoRedo - pushDeleteConditionalEdge 精确还原', () => {
  function seedCondEdges(graphDef: GraphDefinition) {
    const ce0: ConditionalEdgeDef = { from: 'r1', condFuncRef: 'fn1', pathMap: { ok: 'a', fail: 'b' } };
    const ce1: ConditionalEdgeDef = { from: 'r2', condFuncRef: 'fn2', pathMap: { default: 'c' } };
    graphDef.conditionalEdges.push(ce0, ce1);
    return { ce0, ce1 };
  }

  it('删除部分 label：undo 恢复完整 pathMap', () => {
    const { graphDef, ur } = setup();
    const { ce0 } = seedCondEdges(graphDef);

    ur.pushDeleteConditionalEdge(ce0, 0, 'ok');
    expect(graphDef.conditionalEdges[0].pathMap).toEqual({ fail: 'b' });

    ur.undo();
    expect(graphDef.conditionalEdges[0].pathMap).toEqual({ ok: 'a', fail: 'b' });
  });

  it('删空 pathMap 时移除整条 CE；undo 重新插入且兄弟 CE 不被污染', () => {
    const { graphDef, ur } = setup();
    const { ce1 } = seedCondEdges(graphDef);

    // 删除 ce1 的唯一 label → 整条 CE 应被移除
    ur.pushDeleteConditionalEdge(ce1, 1, 'default');
    expect(graphDef.conditionalEdges).toHaveLength(1);
    expect(graphDef.conditionalEdges[0].from).toBe('r1');
    // 兄弟 CE 的 pathMap 必须完好
    expect(graphDef.conditionalEdges[0].pathMap).toEqual({ ok: 'a', fail: 'b' });

    ur.undo();
    expect(graphDef.conditionalEdges).toHaveLength(2);
    expect(graphDef.conditionalEdges[1].from).toBe('r2');
    expect(graphDef.conditionalEdges[1].pathMap).toEqual({ default: 'c' });
    // 兄弟 CE 依然完好
    expect(graphDef.conditionalEdges[0].pathMap).toEqual({ ok: 'a', fail: 'b' });
  });

  it('删空末尾 CE 后 redo 再执行：结果与首次一致', () => {
    const { graphDef, ur } = setup();
    const { ce1 } = seedCondEdges(graphDef);

    ur.pushDeleteConditionalEdge(ce1, 1, 'default');
    ur.undo();
    ur.redo();
    expect(graphDef.conditionalEdges).toHaveLength(1);
    expect(graphDef.conditionalEdges[0].from).toBe('r1');
    expect(graphDef.conditionalEdges[0].pathMap).toEqual({ ok: 'a', fail: 'b' });
  });
});

describe('useGraphUndoRedo - 栈行为', () => {
  it('新命令清空 redo 栈；超出上限丢弃最旧命令', () => {
    const { graphDef, ur } = setup();
    ur.pushAddNode(makeNode('n1'), 0);
    ur.undo();
    expect(ur.canRedo.value).toBe(true);
    ur.pushAddNode(makeNode('n2'), 0);
    expect(ur.canRedo.value).toBe(false);

    for (let i = 0; i < 60; i++) {
      ur.pushAddNode(makeNode(`x${i}`), graphDef.nodes.length);
    }
    // 上限 50：最旧的 pushAddNode(n2) 已被挤出
    let undoCount = 0;
    while (ur.canUndo.value) {
      ur.undo();
      undoCount++;
    }
    expect(undoCount).toBe(50);
  });

  it('clear 清空两栈', () => {
    const { ur } = setup();
    ur.pushAddNode(makeNode('n1'), 0);
    ur.clear();
    expect(ur.canUndo.value).toBe(false);
    expect(ur.canRedo.value).toBe(false);
  });
});
