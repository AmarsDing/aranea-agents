import { describe, expect, it } from 'vitest';
import {
  buildExecNodeStatesFromGraphNodes,
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
