/**
 * OrchestrationSpec v2 — 前端一等类型（M53 §2.2）。
 * 与 definition_json 同构；保存时经 definitionToJSON 写回。
 */
import type { TeamDefinition, TeamDefinitionMember, TeamFailurePolicy } from './types';

export type OrchestrationSpec = TeamDefinition & {
  version: 2;
  runtime_engine: 'graph' | 'native';
  linked_graph_id?: string;
  failure_policy?: TeamFailurePolicy;
  turn_timeout_sec?: number;
  first_byte_timeout_sec?: number;
};

export function toOrchestrationSpec(def: TeamDefinition): OrchestrationSpec {
  const engine = String(def.runtime_engine || 'graph').toLowerCase() === 'native' ? 'native' : 'graph';
  return {
    ...def,
    version: 2,
    runtime_engine: engine,
    turn_timeout_sec: def.timeout_seconds,
  };
}

export function fromOrchestrationSpec(spec: OrchestrationSpec): TeamDefinition {
  const { turn_timeout_sec, ...rest } = spec;
  return {
    ...rest,
    version: spec.version ?? 2,
    timeout_seconds: turn_timeout_sec ?? spec.timeout_seconds,
  };
}

export type { TeamDefinitionMember, TeamFailurePolicy };
