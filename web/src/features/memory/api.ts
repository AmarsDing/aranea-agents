/**
 * 记忆 / L0–L4 / 图谱与进化：**遗留 REST**（`legacyRest.ts` → `/api/v1/...`）。待后端 `memory/v1` 后替换实现。
 * Session 类型请用 `features/session/api` 或 `features/chat/api`。
 */
export type {
  AgentIdentity,
  AgentSkillStat,
  AgentStrategyProfile,
  EvolutionEvent,
  EvolutionMetricsReport,
  EvolutionProposal,
  GraphNeighborhood,
  L0AssemblySegment,
  L0AssemblySnapshot,
  L1Field,
  L1Task,
  MemoryEntity,
  MemoryFact,
  MemoryFactListQuery,
  MemoryFactListResult,
  MemoryRelation
} from "./types";

export {
  getAgentIdentity,
  getAgentStrategy,
  getEvolutionMetrics,
  getMemoryNeighborhood,
  listEvolutionEvents,
  listEvolutionProposals,
  listL0Snapshots,
  listL1Fields,
  listL1Tasks,
  listMemoryEntities,
  listMemoryFacts
} from "./legacyRest";
