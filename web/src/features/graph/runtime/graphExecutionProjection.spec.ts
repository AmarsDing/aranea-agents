import { describe, expect, it } from 'vitest';
import type { NodeDef } from '../types';
import {
  buildExecNodeStatesFromGraphNodes,
  buildGraphRunKanbanNodes,
  buildResumePayload,
  graphNodeStatusToExecStatus,
  parseInterruptPrompt,
  seedGraphNodeStatesFromSteps,
  stepStatusToGraphNodeStatus,
} from './graphExecutionProjection';

describe('graphExecutionProjection', () => {
  it('maps graph node statuses to canvas exec statuses', () => {
    expect(graphNodeStatusToExecStatus('running')).toBe('running');
    expect(graphNodeStatusToExecStatus('interrupted')).toBe('interrupted');
    expect(graphNodeStatusToExecStatus('error')).toBe('failed');
  });

  it('seeds node map from execution steps', () => {
    const nodes = seedGraphNodeStatesFromSteps([
      { nodeId: 'a', stepIndex: 1, inputState: {}, outputState: {}, status: 'completed', error: '', timestamp: '' },
      { nodeId: 'b', stepIndex: 2, inputState: {}, outputState: {}, status: 'running', error: '', timestamp: '' },
    ]);
    expect(nodes.get('a')?.status).toBe('completed');
    expect(nodes.get('b')?.status).toBe('running');
    expect(stepStatusToGraphNodeStatus('waiting_human')).toBe('interrupted');
  });

  it('builds exec node states for canvas', () => {
    const nodes = seedGraphNodeStatesFromSteps([
      { nodeId: 'n1', stepIndex: 0, inputState: {}, outputState: {}, status: 'failed', error: 'boom', timestamp: '' },
    ]);
    const exec = buildExecNodeStatesFromGraphNodes(nodes);
    expect(exec.get('n1')?.status).toBe('failed');
  });

  it('parses interrupt prompt from structured value', () => {
    expect(parseInterruptPrompt({ prompt: 'Confirm?' })).toBe('Confirm?');
    expect(parseInterruptPrompt('plain')).toBe('plain');
  });

  it('builds resume payload with lineage and resume_map', () => {
    const payload = buildResumePayload(
      {
        nodeId: 'confirm',
        interruptKey: 'confirm',
        prompt: 'ok?',
        checkpointId: 'cp-1',
        lineageId: 'line-1',
      },
      true,
    );
    expect(payload).toEqual({
      lineage_id: 'line-1',
      checkpoint_id: 'cp-1',
      resume_map: { confirm: true },
    });
  });
});

// ── M53 Phase 11 F7：GraphRunPage Kanban 视角（team 执行） ──
function mkGraphNode(id: string, agentName = ''): NodeDef {
  return { id, agentName } as NodeDef;
}

describe('buildGraphRunKanbanNodes (M53 Phase 11 F7)', () => {
  it('图定义节点未执行 → waiting/received；step 节点按状态映射 display/phase', () => {
    const nodes = buildGraphRunKanbanNodes(
      [
        { nodeId: 'a', stepIndex: 0, inputState: {}, outputState: {}, status: 'completed', error: '', timestamp: '' },
        { nodeId: 'b', stepIndex: 1, inputState: {}, outputState: {}, status: 'running', error: '', timestamp: '' },
      ],
      new Map(),
      [mkGraphNode('a'), mkGraphNode('b'), mkGraphNode('c')],
    );
    expect(nodes.map((n) => n.node_id)).toEqual(['a', 'b', 'c']);
    expect(nodes[0]).toMatchObject({ status: 'success', display_status: 'success', phase: 'delivered' });
    expect(nodes[1]).toMatchObject({ status: 'running', display_status: 'active', phase: 'doing' });
    expect(nodes[2]).toMatchObject({ status: 'idle', display_status: 'waiting', phase: 'received' });
  });

  it('failed/interrupted 映射：error_message 透传', () => {
    const nodes = buildGraphRunKanbanNodes(
      [
        { nodeId: 'x', stepIndex: 0, inputState: {}, outputState: {}, status: 'failed', error: 'boom', timestamp: '' },
        { nodeId: 'y', stepIndex: 1, inputState: {}, outputState: {}, status: 'interrupted', error: '', timestamp: '' },
      ],
      new Map(),
      [],
    );
    expect(nodes[0]).toMatchObject({ node_id: 'x', status: 'failed', display_status: 'failed', error_message: 'boom' });
    expect(nodes[1]).toMatchObject({ node_id: 'y', status: 'waiting_input', display_status: 'suspended' });
  });

  it('live execNodeStates 覆盖 step 状态（流式推进）', () => {
    const nodes = buildGraphRunKanbanNodes(
      [{ nodeId: 'a', stepIndex: 0, inputState: {}, outputState: {}, status: 'running', error: '', timestamp: '' }],
      new Map([['a', { status: 'completed' }]]),
      [],
    );
    expect(nodes[0]).toMatchObject({ status: 'success', display_status: 'success' });
  });

  it('agent_name 取图定义 agentName，缺省回退 node_id；同节点取最大 stepIndex 的输入/输出快照', () => {
    const nodes = buildGraphRunKanbanNodes(
      [
        {
          nodeId: 'a',
          stepIndex: 0,
          inputState: { q: 'old' },
          outputState: {},
          status: 'running',
          error: '',
          timestamp: '',
        },
        {
          nodeId: 'a',
          stepIndex: 2,
          inputState: { q: 'new' },
          outputState: { r: 'done' },
          status: 'completed',
          error: '',
          timestamp: '',
        },
      ],
      new Map(),
      [mkGraphNode('a', '阿尔法')],
    );
    expect(nodes).toHaveLength(1);
    expect(nodes[0].agent_name).toBe('阿尔法');
    expect(nodes[0].input_preview).toContain('new');
    expect(nodes[0].input_preview).not.toContain('old');
    expect(nodes[0].output_preview).toContain('done');

    const fallback = buildGraphRunKanbanNodes([], new Map(), [mkGraphNode('bare')]);
    expect(fallback[0].agent_name).toBe('bare');
  });

  it('资产删除降级：图定义为空时仅从 steps 渲染节点（按 stepIndex 排序）', () => {
    const nodes = buildGraphRunKanbanNodes(
      [
        { nodeId: 'b', stepIndex: 3, inputState: {}, outputState: {}, status: 'completed', error: '', timestamp: '' },
        { nodeId: 'a', stepIndex: 1, inputState: {}, outputState: {}, status: 'completed', error: '', timestamp: '' },
      ],
      new Map(),
      [],
    );
    expect(nodes.map((n) => n.node_id)).toEqual(['a', 'b']);
  });
});
