import type { PlatformResource, PlatformResourceTreeNode } from "../../features/platform/api";
import type { Agent } from "../../features/agents/api";

export type PromptMode = "complete" | "task" | "minimized" | "none";
export type EvolutionKey = "self_evolve" | "skill_evolve" | "evolution_metrics_enabled" | "evolution_suggestions_enabled";

export type AgentFile = {
  name: string;
  caption: string;
  body: string;
};

export const promptModes = [
  { value: "complete" as PromptMode, label: "完整", caption: "交互聊天 + 完整人格类能力", tokens: "~8K tokens" },
  { value: "task" as PromptMode, label: "任务", caption: "企业自动化、记忆、进化", tokens: "~5K tokens" },
  { value: "minimized" as PromptMode, label: "最小化", caption: "后台任务、核心规则、仅观察", tokens: "~2K tokens" },
  { value: "none" as PromptMode, label: "无", caption: "纯工具调用自动化", tokens: "~2K tokens" }
];

export const descriptionTemplates = [
  { key: "fox", label: "小狐", icon: "pets", text: "温柔、敏捷，擅长把复杂问题拆成清晰步骤。" },
  { key: "programmer", label: "程序员", icon: "code", text: "资深研发工程师，关注架构、代码质量、测试与可维护性。" },
  { key: "support", label: "客服", icon: "support_agent", text: "耐心、清晰，优先解决用户问题并记录上下文。" },
  { key: "writer", label: "写手", icon: "edit_note", text: "擅长品牌文案、结构化写作、润色与多语种表达。" },
  { key: "translator", label: "翻译", icon: "translate", text: "忠实、准确地进行中英互译，并保留术语一致性。" },
  { key: "luo", label: "小罗", icon: "bolt", text: "执行力强，适合任务推进、复盘和状态同步。" },
  { key: "mimi", label: "米米", icon: "auto_awesome", text: "轻松友好，擅长创意发散、陪伴式讨论和灵感整理。" }
];

export const statusOptions = [
  { label: "Active", value: "active" },
  { label: "Inactive", value: "inactive" }
];

export const toolOptions = ["browser", "replace_content", "list_file", "read_file", "save_file", "create_image", "create_video", "stt"];

export const defaultAgentFiles: AgentFile[] = [
  { name: "AGENTS_CORE.md", caption: "通用操作规则、语言跟随、工具保存约束", body: "# AGENTS_CORE\n遵循用户语言，保存变更必须通过文件工具。" },
  { name: "AGENTS_TASK.md", caption: "任务模式规则、memory、cron 与隐私约定", body: "# AGENTS_TASK\n执行企业任务时保持可追踪、可恢复。" },
  { name: "SOUL.md", caption: "人格、语气、价值观；自我进化仅允许调整风格", body: "# SOUL\n保持专业、清晰、克制。" },
  { name: "IDENTITY.md", caption: "对外身份、角色名与边界", body: "# IDENTITY\n我是企业级 Agent。" },
  { name: "USER.md", caption: "Agent 级默认用户上下文（每用户覆盖可后续接入）", body: "# USER\n记录稳定偏好。" },
  { name: "USER_PREDEFINED.md", caption: "预置用户画像与偏好说明", body: "# USER_PREDEFINED\n暂无。" },
  { name: "CAPABILITIES.md", caption: "能力描述、工具使用说明与边界", body: "# CAPABILITIES\n可进行分析、执行和复盘。" },
  { name: "RULE.md", caption: "硬性规则、约束与禁止项", body: "# RULE\n不得越权操作。" },
  { name: "HEARTBEAT.md", caption: "心跳周期注入的检查清单", body: "# 心跳检查清单\n- 检查待处理任务\n- 报告当前状态" }
];

export function promptModeLabel(value: string) {
  return promptModes.find((mode) => mode.value === value)?.label ?? "完整";
}

export function tokenEstimateFor(value: string) {
  return Math.ceil((value || "").length / 4);
}

export function tokenText(value: string) {
  const count = tokenEstimateFor(value);
  return count > 0 ? `估计 ${count} token` : "空";
}

export function formatContext(value: number) {
  if (!value) return "默认 ctx";
  if (value >= 1_000_000) return `${Math.round(value / 1_000_000)}M ctx`;
  if (value >= 1000) return `${Math.round(value / 1000)}K ctx`;
  return `${value} ctx`;
}

export function flattenCategoryPositions(nodes: PlatformResourceTreeNode[], prefix = ""): Array<{ label: string; value: string }> {
  return nodes.flatMap((node) => {
    const label = prefix ? `${prefix} / ${node.name}` : node.name;
    if (!node.children?.length) {
      return [{ label, value: node.id }];
    }
    return flattenCategoryPositions(node.children, label);
  });
}

export function parseAvatarIcon(row: PlatformResource) {
  try {
    return JSON.parse(row.config_json || "{}").icon as string | undefined;
  } catch {
    return undefined;
  }
}

export function selfEvolveEnabled(agent: Agent) {
  if (agent.settings) {
    return agent.settings.self_evolve;
  }
  try {
    return Boolean(JSON.parse(agent.config_json || "{}").self_evolve);
  } catch {
    return false;
  }
}
