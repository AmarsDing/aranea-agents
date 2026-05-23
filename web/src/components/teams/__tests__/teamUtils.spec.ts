import { describe, expect, it } from "vitest";
import { definitionToJSON, parseDefinition } from "../teamUtils";
import type { Team } from "../../../features/teams/types";

describe("teamUtils.parseDefinition", () => {
  it("preserves runtime_engine, failure_policy, and linked_graph_id", () => {
    const team: Team = {
      id: "t1",
      team_key: "demo",
      display_name: "Demo",
      status: "active",
      is_default: false,
      definition_json: JSON.stringify({
        version: 1,
        mode: "parallel",
        runtime_engine: "graph",
        team_graph_runtime: true,
        linked_graph_id: "g-1",
        failure_policy: { default: "retry_then_block", parallel_fail: "continue" },
        members: [{ agent_id: "a1", role: "worker", name: "W", enabled: true, sort_order: 10 }]
      }),
      app_name: "demo",
      linked_graph_id: "g-team-col",
      has_active_run: false,
      created_at: "",
      updated_at: "",
      deleted_at: ""
    };

    const def = parseDefinition(team);
    expect(def.runtime_engine).toBe("graph");
    expect(def.team_graph_runtime).toBe(true);
    expect(def.linked_graph_id).toBe("g-1");
    expect(def.failure_policy?.parallel_fail).toBe("continue");

    const json = JSON.parse(definitionToJSON(def)) as Record<string, unknown>;
    expect(json.runtime_engine).toBe("graph");
    expect(json.team_graph_runtime).toBe(true);
    expect(json.failure_policy).toEqual({ default: "retry_then_block", parallel_fail: "continue" });
  });
});
