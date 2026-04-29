/**
 * 记忆 / L0–L4 / 图谱与进化：`/memory/…`、`/agents/…/identity|MCP`、进化相关路径（经 clientLegacy）。
 * 待 `memory/v1` Kratos 后换实现；与 Session 类型请用 `features/chat/api`（Playbook §6 行 7）。
 */
export {
  getAgentIdentity,
  getAgentStrategy,
  getEvolutionMetrics,
  getMemoryNeighborhood,
  listEvolutionEvents,
  listEvolutionProposals,
  listL0Snapshots,
  listL1Tasks,
  listMemoryEntities,
  listMemoryFacts,
  type AgentIdentity,
  type AgentStrategyProfile,
  type EvolutionEvent,
  type EvolutionMetricsReport,
  type EvolutionProposal,
  type GraphNeighborhood,
  type L0AssemblySegment,
  type L0AssemblySnapshot,
  type L1Task,
  type MemoryEntity,
  type MemoryFact,
  type MemoryFactListQuery,
  type MemoryFactListResult
} from "../../services/clientLegacy";
