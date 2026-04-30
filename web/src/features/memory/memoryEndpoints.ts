/**
 * `memory/v1` HTTP 路由（相对网关 Origin，无前导 `/`）。
 * 实际请求由 {@link createMemoryService} / `kratosApi` 发起；此处供文档、调试或手写 `requestHandler` 复用。
 */
export const memoryEndpoints = {
  listL0Snapshots: (sessionId: string) => `v1/sessions/${encodeURIComponent(sessionId)}/l0/snapshots`,
  listL1Tasks: (sessionId: string) => `v1/sessions/${encodeURIComponent(sessionId)}/l1/tasks`,
  listL1Fields: (sessionId: string, taskId: string) =>
    `v1/sessions/${encodeURIComponent(sessionId)}/l1/tasks/${encodeURIComponent(taskId)}/fields`,
  listMemoryFacts: () => "v1/memory/l3/facts",
  listMemoryEntities: () => "v1/memory/l4/entities",
  getMemoryNeighborhood: (centerId: string) =>
    `v1/memory/l4/entities/${encodeURIComponent(centerId)}/neighborhood`,
  getAgentIdentity: (agentId: string) => `v1/agents/${encodeURIComponent(agentId)}/identity`,
  getAgentStrategy: (agentId: string) => `v1/agents/${encodeURIComponent(agentId)}/strategy`,
  listEvolutionProposals: (agentId: string) => `v1/agents/${encodeURIComponent(agentId)}/evolution/proposals`,
  listEvolutionEvents: (agentId: string) => `v1/agents/${encodeURIComponent(agentId)}/evolution/events`,
  getEvolutionMetrics: (agentId: string) => `v1/agents/${encodeURIComponent(agentId)}/evolution/metrics`
} as const;
