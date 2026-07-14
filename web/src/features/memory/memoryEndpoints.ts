/**
 * `memory/v1` HTTP 路由（相对网关 Origin，无前导 `/`）。
 * 实际请求由 {@link createMemoryService} / `kratosApi` 发起；此处供文档、调试或手写 `requestHandler` 复用。
 */
export const memoryEndpoints = {
  listL0Snapshots: (sessionId: string) => `v1/sessions/${encodeURIComponent(sessionId)}/l0/snapshots`,
  listL1Tasks: (sessionId: string) => `v1/sessions/${encodeURIComponent(sessionId)}/l1/tasks`,
  listL1Fields: (sessionId: string, taskId: string) =>
    `v1/sessions/${encodeURIComponent(sessionId)}/l1/tasks/${encodeURIComponent(taskId)}/fields`,
  listMemoryFacts: () => 'v1/memory/l3/facts',
  listMemoryEntities: () => 'v1/memory/l4/entities',
  getMemoryNeighborhood: (centerId: string) => `v1/memory/l4/entities/${encodeURIComponent(centerId)}/neighborhood`,
  spreadingActivation: () => 'v1/memory/l4/spreading-activation',
  getAgentIdentity: (agentId: string) => `v1/agents/${encodeURIComponent(agentId)}/identity`,
  getAgentStrategy: (agentId: string) => `v1/agents/${encodeURIComponent(agentId)}/strategy`,
  listEvolutionProposals: (agentId: string) => `v1/agents/${encodeURIComponent(agentId)}/evolution/proposals`,
  listEvolutionEvents: (agentId: string) => `v1/agents/${encodeURIComponent(agentId)}/evolution/events`,
  appendEvolutionEvent: (agentId: string) => `v1/agents/${encodeURIComponent(agentId)}/evolution/events`,
  getEvolutionMetrics: (agentId: string) => `v1/agents/${encodeURIComponent(agentId)}/evolution/metrics`,
  upsertMemoryFact: () => 'v1/memory/l3/facts',
  listCascadeProposals: (agentId: string) => `v1/memory/cascade/proposals?agent_id=${encodeURIComponent(agentId)}`,
  approveCascadeProposal: (id: string) => `v1/memory/cascade/proposals/${encodeURIComponent(id)}/approve`,
  rejectCascadeProposal: (id: string) => `v1/memory/cascade/proposals/${encodeURIComponent(id)}/reject`,
  previewCascadeApprove: (id: string) => `v1/memory/cascade/proposals/${encodeURIComponent(id)}/preview`,
  getCascadeSagaSteps: (proposalId: string) =>
    `v1/memory/cascade/proposals/${encodeURIComponent(proposalId)}/saga-steps`,
  retryCascadeApprove: (id: string) => `v1/memory/cascade/proposals/${encodeURIComponent(id)}/retry`,
  compensateCascadeApprove: (id: string) => `v1/memory/cascade/proposals/${encodeURIComponent(id)}/compensate`,
  debugMemoryRecall: () => 'v1/memory/recall/debug',
  compositeSearchMemories: () => 'v1/memory/search/composite',
  getMemoryWorkerStatus: () => 'v1/memory/worker/status',
  getMemoryPlatformSettings: () => 'v1/memory/platform/settings',
  updateMemoryPlatformSettings: () => 'v1/memory/platform/settings',
} as const;
