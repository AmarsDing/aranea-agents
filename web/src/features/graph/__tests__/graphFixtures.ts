import type { GraphDefinition, NodeDef } from '../types';

/** Complete independent defaults keep UI fixtures checked against the domain contract. */
export function graphNodeFixture(overrides: Partial<NodeDef> = {}): NodeDef {
  return {
    id: 'node',
    type: 'function',
    funcRef: '',
    description: '',
    instruction: '',
    modelName: '',
    toolNames: [],
    agentName: '',
    interruptBefore: false,
    interruptAfter: false,
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
    ...overrides,
  };
}

export function graphDefinitionFixture(overrides: Partial<GraphDefinition> = {}): GraphDefinition {
  return {
    id: 'graph',
    name: 'Graph',
    description: '',
    version: 1,
    nodes: [],
    edges: [],
    conditionalEdges: [],
    subgraphs: [],
    entryPoint: '',
    finishPoint: '',
    stateFields: [],
    enableCheckpoint: true,
    executionEngine: 'bsp',
    interruptBefore: [],
    interruptAfter: [],
    metadata: {},
    sortOrder: 0,
    createdAt: '',
    updatedAt: '',
    ...overrides,
  };
}
