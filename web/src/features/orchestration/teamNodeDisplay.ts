import type { NodeDef } from "../graph/types";
import type { CompileTeamGraphResult } from "./compileApi";
import type { TeamDefinition } from "../teams/types";

export type TeamNodeDisplay = {
  nodeId: string;
  displayName: string;
  agentKey: string;
  role: string;
  roleLabel: string;
  responsibility: string;
  /** Design-time hint for upstream input (from mode / topology). */
  inputHint: string;
  /** Design-time hint for downstream output. */
  outputHint: string;
};

const ROLE_INPUT_HINTS: Record<string, string> = {
  coordinator: "接收用户任务，分解并分派给成员",
  worker: "接收协调者或上游节点的任务上下文",
  synthesizer: "汇聚各成员输出，生成最终答复",
  critic: "评审上游产出，给出通过/修改意见",
};

const ROLE_OUTPUT_HINTS: Record<string, string> = {
  coordinator: "任务计划、成员分派指令",
  worker: "阶段性成果、工具调用结果",
  synthesizer: "团队最终交付物",
  critic: "评审结论、修改建议",
};

const ROLE_LABELS: Record<string, string> = {
  coordinator: "协调",
  worker: "执行",
  synthesizer: "汇总",
  critic: "评审",
  generator: "生成",
};

function roleLabel(role: string): string {
  const key = role.trim().toLowerCase();
  return ROLE_LABELS[key] ?? (role || "成员");
}

function findMemberForNode(
  nodeId: string,
  agentKey: string,
  definition: TeamDefinition | null,
) {
  const members = definition?.members ?? [];
  const graphNode = definition?.graph?.nodes?.find((n) => n.id === nodeId);
  if (graphNode?.agent_id) {
    const byId = members.find((m) => m.agent_id === graphNode.agent_id);
    if (byId) return byId;
  }
  if (agentKey) {
    const byKey = members.find((m) => m.agent_id === agentKey || m.name === agentKey);
    if (byKey) return byKey;
  }
  const sortMatch = /^member-(\d+)$/.exec(nodeId);
  if (sortMatch) {
    const order = Number(sortMatch[1]);
    return members.find((m) => m.sort_order === order) ?? members[order - 1];
  }
  return undefined;
}

export function resolveTeamNodeDisplay(
  node: NodeDef,
  compiled: CompileTeamGraphResult | null,
  definition: TeamDefinition | null,
): TeamNodeDisplay {
  const compiledNode = compiled?.nodes.find((n) => n.id === node.id);
  const member = findMemberForNode(node.id, node.agentName, definition);
  const role = (compiledNode?.role || node.requiredRole || member?.role || "worker").trim().toLowerCase();
  const agentKey = compiledNode?.agentName || node.agentName || member?.agent_id || "";
  const displayName =
    compiledNode?.agentDisplayName?.trim() ||
    compiledNode?.description?.trim() ||
    member?.name?.trim() ||
    node.description?.trim() ||
    agentKey ||
    node.id;
  const responsibility =
    compiledNode?.taskPrompt?.trim() ||
    node.instruction?.trim() ||
    member?.task_prompt?.trim() ||
    member?.name?.trim() ||
    ROLE_INPUT_HINTS[role] ||
    "执行编排分配的子任务";

  return {
    nodeId: node.id,
    displayName,
    agentKey,
    role,
    roleLabel: roleLabel(role),
    responsibility,
    inputHint: ROLE_INPUT_HINTS[role] ?? "接收上游 state / 协调者输入",
    outputHint: ROLE_OUTPUT_HINTS[role] ?? "写入 state，传递给下游节点",
  };
}

export function graphNodeDisplayLabel(node: NodeDef): string {
  if (node.type === "agent") {
    return node.description?.trim() || node.agentName?.trim() || node.id;
  }
  return node.agentName?.trim() || node.description?.trim() || node.id;
}
