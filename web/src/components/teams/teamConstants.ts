/**
 * Team UI 常量：状态映射、模式/角色/引擎/失败策略选项、模板定义。
 * 与 `components/teams/*.vue` 共址，见 aranea-frontend-guide SKILL §3.3 路径硬性约定。
 */

export const teamStatusMap: Record<string, { label: string; color: string }> = {
  pending: { label: '待执行', color: 'warning' },
  running: { label: '执行中', color: 'positive' },
  completed: { label: '已完成', color: 'blue' },
  failed: { label: '失败', color: 'negative' },
  cancelled: { label: '已取消', color: 'grey' },
  interrupted: { label: '已中断', color: 'orange' },
  archived: { label: '已归档', color: 'grey' },
  active: { label: '活跃', color: 'positive' },
};

export const modeOptions = [
  { label: '顺序 sequential', value: 'sequential' },
  { label: '并行 parallel', value: 'parallel' },
  { label: '主控 coordinator', value: 'coordinator' },
  { label: '生成评审 critic_loop', value: 'critic_loop' },
  {
    label: '群智 adaptive（Swarm）',
    value: 'adaptive',
    description: '成员间 transfer_to_agent 协作；后端与 swarm 共用 Swarm 运行时。',
  },
];

/**
 * Team 可选状态选项，对齐后端 team_graph_constants.go 状态机。
 * - pending/running/completed/failed/cancelled/interrupted/archived 为后端持久化状态
 * - 编辑时仅允许合法转换（参考 validStatusTransitions）
 */
export const statusOptions = (
  ['pending', 'running', 'completed', 'failed', 'cancelled', 'interrupted', 'archived'] as const
).map((value) => ({ label: teamStatusMap[value]?.label ?? value, value }));

/** 后端 ValidTeamStatusTransition 的前端镜像 */
export const validStatusTransitions: Record<string, string[]> = {
  pending: ['running', 'cancelled'],
  running: ['completed', 'failed', 'cancelled', 'interrupted'],
  interrupted: ['running'],
  completed: ['archived'],
  failed: ['archived'],
  cancelled: ['archived'],
  archived: [],
};

/** 判断状态转换是否合法 */
export function isValidStatusTransition(from: string, to: string): boolean {
  if (from === to) return true;
  return validStatusTransitions[from]?.includes(to) ?? false;
}

export const roleOptions = ['worker', 'coordinator', 'synthesizer', 'generator', 'critic'].map((value) => ({
  label: value,
  value,
}));

export type TeamTemplateKey = 'sequential' | 'parallel_experts' | 'critic_loop' | 'coordinator';

export const teamTemplateOptions: Array<{ label: string; value: TeamTemplateKey; description: string }> = [
  { label: '顺序协作', value: 'sequential', description: '多个 worker 按顺序接力处理任务。' },
  {
    label: '并行专家组',
    value: 'parallel_experts',
    description: '前若干成员并行产出；列表中的最后一位 Agent 固定为汇总角色（与专家槽位不同实例）。',
  },
  {
    label: '生成评审',
    value: 'critic_loop',
    description: 'generator 与 critic 顺序迭代；迭代次数取自编排里的 critic_loop.max_iterations。',
  },
  {
    label: '主控分派',
    value: 'coordinator',
    description: '成员顺序执行；当前运行时为带迭代的上屏顺序链（非独立并行拓扑），适合分步接力。',
  },
];

export const runtimeEngineOptions = [
  {
    label: 'Graph（默认，GraphAgent）',
    value: 'graph',
    description: 'CompileToGraphRuntimeConfig → GraphAgent；生产推荐。',
  },
  {
    label: 'Native（BuildTRPCTeam）',
    value: 'native',
    description: '按 mode 分发 Chain/Parallel/Swarm；仅 fallback 或调试。',
  },
];

export const failureDefaultOptions = [
  { label: '重试后阻塞 retry_then_block', value: 'retry_then_block' },
  { label: '跳过 skip', value: 'skip' },
  { label: '快速失败 fail_fast', value: 'fail_fast' },
];

export const parallelFailOptions = [
  { label: '继续 continue（分支失败可跳过）', value: 'continue' },
  { label: '中止 abort', value: 'abort' },
];

export const failureOnErrorOptions = [
  { label: '暂停等审核 await_review', value: 'await_review' },
  { label: '终止 halt', value: 'halt' },
];

export const BuiltinIndustryId = '__builtin__';
export const PresetIndustryId = '__preset__';
