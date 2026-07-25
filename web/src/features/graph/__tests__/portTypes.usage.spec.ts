// web/src/features/graph/__tests__/portTypes.usage.spec.ts
import { describe, it, expect } from 'vitest';
import { buildFieldUsageMap } from '../portTypes';
import type { NodeDef } from '../types';

function node(partial: Partial<NodeDef>): NodeDef {
  return {
    id: 'n1',
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
    assignmentMode: '',
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
    ...partial,
  };
}

describe('buildFieldUsageMap - R2-8 usageMap 派生', () => {
  it('llm 节点：instruction 模板字段 → reads；last_response → writes', () => {
    const nodes = [node({ id: 'llm1', type: 'llm', instruction: '总结 ${topic} 和 ${notes}' })];
    const map = buildFieldUsageMap(nodes);
    expect(map.get('topic')).toEqual({ reads: ['llm1'], writes: [] });
    expect(map.get('notes')).toEqual({ reads: ['llm1'], writes: [] });
    expect(map.get('last_response')).toEqual({ reads: [], writes: ['llm1'] });
  });

  it('agent 节点：inputMapper 值 → reads；outputMapper 键 → writes', () => {
    const nodes = [
      node({
        id: 'a1',
        type: 'agent',
        inputMapperJson: '{"q": "user_query"}',
        outputMapperJson: '{"final_answer": "result"}',
      }),
    ];
    const map = buildFieldUsageMap(nodes);
    expect(map.get('user_query')).toEqual({ reads: ['a1'], writes: [] });
    expect(map.get('final_answer')).toEqual({ reads: [], writes: ['a1'] });
  });

  it('多节点读写同一字段时合并 nodeId 列表', () => {
    const nodes = [
      node({ id: 'w1', type: 'agent', outputMapperJson: '{"shared": "x"}' }),
      node({ id: 'r1', type: 'llm', instruction: '读取 ${shared}' }),
      node({ id: 'r2', type: 'agent', inputMapperJson: '{"in": "shared"}' }),
    ];
    const map = buildFieldUsageMap(nodes);
    expect(map.get('shared')).toEqual({ reads: ['r1', 'r2'], writes: ['w1'] });
  });

  it('function/tool/router/join/hitl 节点不产生引用', () => {
    const nodes = [
      node({ id: 'f1', type: 'function' }),
      node({ id: 't1', type: 'tool' }),
      node({ id: 'ro1', type: 'router' }),
      node({ id: 'j1', type: 'join' }),
      node({ id: 'h1', type: 'hitl' }),
    ];
    const map = buildFieldUsageMap(nodes);
    expect(map.size).toBe(0);
  });

  it('非法 mapper JSON 静默忽略', () => {
    const nodes = [node({ id: 'a1', type: 'agent', inputMapperJson: '{bad json', outputMapperJson: '[1,2]' })];
    const map = buildFieldUsageMap(nodes);
    expect(map.size).toBe(0);
  });
});
