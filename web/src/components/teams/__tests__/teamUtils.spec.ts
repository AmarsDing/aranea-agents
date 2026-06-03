import { describe, expect, it } from "vitest";
import { definitionToJSON, groupTeamsByIndustry, inferTeamIndustryId, parseDefinition } from "../teamUtils";
import type { Team } from "../../../features/teams/types";
import type { Agent } from "../../../features/agents/types";
import type { PlatformResourceTreeNode } from "../../../features/platform/types";

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

describe("teamUtils.groupTeamsByIndustry", () => {
  const categoryTree = [
    {
      id: "ind-1",
      resource: "taxonomy-nodes",
      key: "finance",
      name: "金融",
      description: "",
      status: "active",
      enabled: true,
      sort_order: 10,
      parent_id: "",
      level: "industry",
      agent_id: "",
      provider: "",
      model: "",
      config_json: "",
      metadata_json: "",
      children: [
        {
          id: "dep-1",
          resource: "taxonomy-nodes",
          key: "research",
          name: "研究部",
          description: "",
          status: "active",
          enabled: true,
          sort_order: 10,
          parent_id: "ind-1",
          level: "department",
          agent_id: "",
          provider: "",
          model: "",
          config_json: "",
          metadata_json: "",
          children: [
            {
              id: "pos-1",
              resource: "taxonomy-nodes",
              key: "analyst",
              name: "分析师",
              description: "",
              status: "active",
              enabled: true,
              sort_order: 10,
              parent_id: "dep-1",
              level: "position",
              agent_id: "",
              provider: "",
              model: "",
              config_json: "",
              metadata_json: "",
              children: []
            }
          ]
        }
      ]
    }
  ] as unknown as PlatformResourceTreeNode[];

  const agents: Agent[] = [
    {
      id: "a1",
      taxonomy_position_id: "pos-1"
    } as Agent
  ];

  const team: Team = {
    id: "t1",
    team_key: "demo",
    display_name: "Demo",
    status: "active",
    is_default: false,
    definition_json: JSON.stringify({
      version: 1,
      mode: "sequential",
      members: [{ agent_id: "a1", role: "worker", name: "W", enabled: true, sort_order: 10 }]
    }),
    app_name: "demo",
    linked_graph_id: "",
    has_active_run: false,
    created_at: "",
    updated_at: "",
    deleted_at: ""
  };

  it("infers industry from member agents", () => {
    expect(inferTeamIndustryId(team, agents, categoryTree)).toBe("ind-1");
    const groups = groupTeamsByIndustry([team], agents, categoryTree);
    expect(groups).toHaveLength(1);
    expect(groups[0]?.label).toBe("金融");
    expect(groups[0]?.teams).toHaveLength(1);
  });

  it("still shows teams under disabled industries", () => {
    const disabledTree: PlatformResourceTreeNode[] = [
      {
        ...categoryTree[0]!,
        enabled: false
      }
    ];
    const groups = groupTeamsByIndustry([team], agents, disabledTree);
    expect(groups).toHaveLength(1);
    expect(groups[0]?.label).toBe("金融（已停用）");
    expect(groups[0]?.teams).toHaveLength(1);
  });
});
