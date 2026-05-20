import type { A2AAgentCard, A2AAuditEntry } from "./types";

/** Maps API agent card payload to UI model (normalizes capabilities). */
export function mapAgentCard(raw: Record<string, unknown>): A2AAgentCard {
  const caps = Array.isArray(raw.capabilities) ? raw.capabilities : [];
  return {
    agent_id: String(raw.agent_id ?? ""),
    display_name: String(raw.display_name ?? raw.agent_id ?? ""),
    workspace: String(raw.workspace ?? ""),
    enabled: Boolean(raw.enabled),
    capabilities: caps.map((c) => {
      const item = c as Record<string, unknown>;
      return {
        name: String(item.name ?? ""),
        description: String(item.description ?? ""),
        input_schema_json: String(item.input_schema_json ?? "{}"),
        output_schema_json: String(item.output_schema_json ?? "{}")
      };
    }),
    updated_at: String(raw.updated_at ?? "")
  };
}

/** Maps audit log row from API. */
export function mapAuditEntry(raw: Record<string, unknown>): A2AAuditEntry {
  return {
    id: String(raw.id ?? ""),
    invoke_id: String(raw.invoke_id ?? ""),
    caller_agent_id: String(raw.caller_agent_id ?? ""),
    callee_agent_id: String(raw.callee_agent_id ?? ""),
    capability: String(raw.capability ?? ""),
    status: String(raw.status ?? ""),
    duration_ms: Number(raw.duration_ms ?? 0),
    workspace: String(raw.workspace ?? ""),
    created_at: String(raw.created_at ?? "")
  };
}
