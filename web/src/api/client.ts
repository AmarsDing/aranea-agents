/**
 * Legacy path-alias barrel for `@/api/client`; prefer `features/<domain>/api` and `create*Service` from `services`.
 */export type {
  Team,
  TeamDefinition,
  TeamDefinitionGraphEdge,
  TeamDefinitionGraphNode,
  TeamDefinitionMember,
  TeamRun,
  TeamRunStep,
  TeamRunEvent
} from "../features/teams/types";
export { subscribeTeamRunEvents } from "../features/teams/api";

export type {
  Session,
  SessionSearchQuery,
  SessionListResult,
  SessionTimelineItem,
  SessionTimelineSummary,
  SessionTimeline
} from "../features/session/api";

export type { L0AssemblySegment, L0AssemblySnapshot, L1Task } from "../features/memory/types";

export { api, syncApiBaseURL } from "./http";
