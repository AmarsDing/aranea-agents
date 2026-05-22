/**
 * Team 展示用纯函数（无网络）。与 `components/teams/*.vue` 共址，见 vue-design.md §2 路径硬性约定。
 * 类型来自 `features/teams/api`（仅 type import）。
 */
import type { Agent } from "../../features/agents/types";
import type { Team, TeamDefinition, TeamDefinitionGraphNode } from "../../features/teams/types";

export const modeOptions = [
  { label: "顺序 sequential", value: "sequential" },
  { label: "并行 parallel", value: "parallel" },
  { label: "主控 coordinator", value: "coordinator" },
  { label: "生成评审 critic_loop", value: "critic_loop" },
  {
    label: "群智 adaptive（Swarm）",
    value: "adaptive",
    description: "成员间 transfer_to_agent 协作；后端与 swarm 共用 Swarm 运行时。"
  }
];

export const statusOptions = ["draft", "active", "archived"].map((value) => ({ label: value, value }));

export const roleOptions = ["worker", "coordinator", "synthesizer", "generator", "critic"].map((value) => ({ label: value, value }));

export type TeamTemplateKey = "sequential" | "parallel_experts" | "critic_loop" | "coordinator";

export const teamTemplateOptions: Array<{ label: string; value: TeamTemplateKey; description: string }> = [
  { label: "顺序协作", value: "sequential", description: "多个 worker 按顺序接力处理任务。" },
  {
    label: "并行专家组",
    value: "parallel_experts",
    description: "前若干成员并行产出；列表中的最后一位 Agent 固定为汇总角色（与专家槽位不同实例）。"
  },
  { label: "生成评审", value: "critic_loop", description: "generator 与 critic 顺序迭代；迭代次数取自编排里的 critic_loop.max_iterations。" },
  {
    label: "主控分派",
    value: "coordinator",
    description: "成员顺序执行；当前运行时为带迭代的上屏顺序链（非独立并行拓扑），适合分步接力。"
  }
];

export function defaultDefinition(): TeamDefinition {
  const definition: TeamDefinition = {
    version: 1,
    description: "",
    mode: "sequential",
    max_concurrency: 2,
    timeout_seconds: 600,
    loop_max_iterations: 0,
    intent_anchor_agent_id: "",
    a2a: defaultA2AConfig(),
    members: [],
    critic_loop: { max_iterations: 3, score_threshold: 0.8 }
  };
  return withGraph(definition);
}

export function definitionFromTemplate(template: TeamTemplateKey, agents: Agent[]): TeamDefinition {
  const base = defaultDefinition();
  if (template === "parallel_experts") {
    const synthId = agents.length ? agents[agents.length - 1].id : "";
    return withGraph({
      ...base,
      description:
        "并行专家组：靠前槽位的专家并行产出；团队 Agent 列表中的最后一位负责汇总（与并行槽位区分）。",
      mode: "parallel",
      max_concurrency: Math.max(2, Math.min(3, agents.length || 2)),
      synthesizer_agent_id: synthId,
      members: [
        templateMember(agents, 0, "worker", "专家 A", 10),
        templateMember(agents, 1, "worker", "专家 B", 20),
        templateMember(agents, 2, "worker", "专家 C", 30)
      ].filter((member) => member.agent_id)
    });
  }
  if (template === "critic_loop") {
    return withGraph({
      ...base,
      description: "生成评审：generator 先产出初稿，critic 评审后按需修订。",
      mode: "critic_loop",
      max_concurrency: 1,
      critic_loop: { max_iterations: 2, score_threshold: 0.8 },
      members: [
        templateMember(agents, 0, "generator", "生成者", 10),
        templateMember(agents, 1, "critic", "评审者", 20)
      ].filter((member) => member.agent_id)
    });
  }
  if (template === "coordinator") {
    return withGraph({
      ...base,
      description: "主控分派：coordinator 先拆解任务，worker 按计划完成分工。",
      mode: "coordinator",
      max_concurrency: 2,
      loop_max_iterations: 1,
      members: [
        templateMember(agents, 0, "coordinator", "主控", 10),
        templateMember(agents, 1, "worker", "执行者 A", 20),
        templateMember(agents, 2, "worker", "执行者 B", 30)
      ].filter((member) => member.agent_id)
    });
  }
  return withGraph({
    ...base,
    description: "顺序协作：多个 Agent 按顺序接力，上一个成员输出作为下一个成员输入。",
    mode: "sequential",
    max_concurrency: 1,
    members: [
      templateMember(agents, 0, "worker", "第一棒", 10),
      templateMember(agents, 1, "worker", "第二棒", 20)
    ].filter((member) => member.agent_id)
  });
}

function templateMember(agents: Agent[], index: number, role: string, fallbackName: string, sortOrder: number) {
  const agent = pickAgent(agents, index);
  return {
    agent_id: agent?.id ?? "",
    role,
    name: agent?.display_name || fallbackName,
    enabled: true,
    sort_order: sortOrder
  };
}

function pickAgentID(agents: Agent[], index: number) {
  return pickAgent(agents, index)?.id ?? "";
}

function pickAgent(agents: Agent[], index: number) {
  if (agents.length === 0) return undefined;
  return agents[Math.min(index, agents.length - 1)];
}

export function parseDefinition(team: Team): TeamDefinition {
  try {
    const parsed = JSON.parse(team.definition_json || "{}") as TeamDefinition;
    return withGraph({
      version: parsed.version || 1,
      description: parsed.description || "",
      mode: parsed.mode || "sequential",
      max_concurrency: parsed.max_concurrency || 2,
      timeout_seconds: parsed.timeout_seconds || 600,
      loop_max_iterations: typeof parsed.loop_max_iterations === "number" ? parsed.loop_max_iterations : 0,
      intent_anchor_agent_id: typeof parsed.intent_anchor_agent_id === "string" ? parsed.intent_anchor_agent_id : "",
      a2a: parsed.a2a || defaultA2AConfig(),
      members: Array.isArray(parsed.members) ? parsed.members : [],
      graph: parsed.graph,
      synthesizer_agent_id: parsed.synthesizer_agent_id,
      critic_loop: parsed.critic_loop || { max_iterations: 3, score_threshold: 0.8 }
    });
  } catch {
    return defaultDefinition();
  }
}

export function defaultA2AConfig() {
  return {
    enabled: true,
    envelope_version: "a2a.v1",
    message_format: "markdown_json",
    include_trace: true,
    max_payload_chars: 6000
  };
}

export function agentName(agents: Agent[], id: string) {
  return agents.find((agent) => agent.id === id)?.display_name || id || "未选择 Agent";
}

export function memberIcon(role: string) {
  return ({ coordinator: "route", synthesizer: "merge_type", generator: "edit_note", critic: "fact_check", worker: "smart_toy" } as Record<string, string>)[role] || "smart_toy";
}

export function topologyNodesFromDefinition(def: TeamDefinition) {
  const mode = def.mode || "sequential";
  if (mode === "adaptive") return [{ icon: "auto_awesome", label: "分析任务" }, { icon: "route", label: "选择拓扑" }, { icon: "play_arrow", label: "执行" }];
  if (mode === "parallel") return [{ icon: "call_split", label: "并行分派" }, { icon: "groups", label: "Worker" }, { icon: "merge_type", label: "汇总" }];
  if (mode === "coordinator") return [{ icon: "route", label: "主控拆分" }, { icon: "smart_toy", label: "成员执行" }, { icon: "summarize", label: "总结" }];
  if (mode === "critic_loop") return [{ icon: "edit_note", label: "生成" }, { icon: "fact_check", label: "评审" }, { icon: "loop", label: "迭代" }];
  return [{ icon: "looks_one", label: "顺序 1" }, { icon: "arrow_forward", label: "传递" }, { icon: "flag", label: "最终输出" }];
}

export function topologyNodes(team: Team) {
  return topologyNodesFromDefinition(parseDefinition(team));
}

export function withGraph(definition: TeamDefinition): TeamDefinition {
  return {
    ...definition,
    graph: definition.graph?.nodes?.length ? definition.graph : buildGraphFromDefinition(definition)
  };
}

export function buildGraphFromDefinition(def: TeamDefinition): NonNullable<TeamDefinition["graph"]> {
  const members = [...(def.members || [])].sort((a, b) => a.sort_order - b.sort_order);
  const layout = graphLayoutForMode(def.mode || "sequential");
  const nodes: TeamDefinitionGraphNode[] = [
    { id: "start", type: "start", label: "开始", x: 0, y: 80 },
    ...members.map((member, index) => ({
      id: graphMemberID(member, index),
      type: "agent",
      label: member.name || member.role || `Agent ${index + 1}`,
      agent_id: member.agent_id,
      role: member.role,
      x: graphX(def.mode || "sequential", index, members.length),
      y: graphY(def.mode || "sequential", index, members.length)
    })),
    { id: "end", type: "end", label: "结束", x: graphEndX(def.mode || "sequential", members.length), y: 80 }
  ];
  return { version: 1, layout, nodes, edges: buildGraphEdges(def.mode || "sequential", members) };
}

function buildGraphEdges(mode: string, members: TeamDefinition["members"]) {
  const ids = members.map(graphMemberID);
  if (ids.length === 0) return [{ id: "start-end", source: "start", target: "end", label: "no members" }];
  if (mode === "adaptive") {
    return [
      { id: "start-adaptive", source: "start", target: ids[0], label: "select topology" },
      ...ids.slice(0, -1).map((id, index) => ({ id: `${id}-${ids[index + 1]}`, source: id, target: ids[index + 1], label: "candidate" })),
      { id: `${ids[ids.length - 1]}-end`, source: ids[ids.length - 1], target: "end", label: "final" }
    ];
  }
  if (mode === "parallel") {
    return [
      ...ids.map((id) => ({ id: `start-${id}`, source: "start", target: id, label: "fan out" })),
      ...ids.map((id) => ({ id: `${id}-end`, source: id, target: "end", label: "join" }))
    ];
  }
  if (mode === "critic_loop") {
    const edges = [{ id: `start-${ids[0]}`, source: "start", target: ids[0], label: "draft" }];
    for (let i = 0; i < ids.length - 1; i++) edges.push({ id: `${ids[i]}-${ids[i + 1]}`, source: ids[i], target: ids[i + 1], label: i === 0 ? "review" : "revise" });
    if (ids.length > 1) edges.push({ id: `${ids[ids.length - 1]}-${ids[0]}-loop`, source: ids[ids.length - 1], target: ids[0], label: "optional loop" });
    edges.push({ id: `${ids[ids.length - 1]}-end`, source: ids[ids.length - 1], target: "end", label: "approved" });
    return edges;
  }
  return ["start", ...ids, "end"].slice(0, -1).map((source, index, chain) => {
    const target = index === chain.length - 1 ? "end" : chain[index + 1];
    return { id: `${source}-${target}`, source, target, label: mode === "coordinator" && index === 0 ? "plan" : "next" };
  });
}

function graphMemberID(member: TeamDefinition["members"][number], index: number) {
  return `member-${member.sort_order || index + 1}`;
}

function graphLayoutForMode(mode: string) {
  if (mode === "adaptive") return "adaptive";
  if (mode === "parallel") return "parallel";
  if (mode === "critic_loop") return "loop";
  if (mode === "coordinator") return "coordinator";
  return "linear";
}

function graphX(mode: string, index: number, total: number) {
  if (mode === "parallel") return 160;
  return 160 + index * 150;
}

function graphY(mode: string, index: number, total: number) {
  if (mode !== "parallel") return 80;
  const offset = (index - (total - 1) / 2) * 74;
  return 80 + offset;
}

function graphEndX(mode: string, total: number) {
  if (mode === "parallel") return 360;
  return 160 + Math.max(total, 1) * 150;
}

export function formatDate(value: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}
