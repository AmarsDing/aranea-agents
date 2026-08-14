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

/**
 * 编排模式中文映射（UI 展示用；value 保持后端枚举）。
 * 合法 API 值共 6 个（sequential / parallel / coordinator / critic_loop / swarm / adaptive）。
 * swarm 与 adaptive 共用 Swarm 运行时，均展示为「群智」。
 */
export const teamModeMap: Record<string, string> = {
  sequential: '顺序',
  parallel: '并行',
  coordinator: '主控',
  critic_loop: '生成评审',
  adaptive: '群智',
  swarm: '群智',
};

export function teamModeLabel(value?: string): string {
  const key = String(value || '')
    .trim()
    .toLowerCase();
  return teamModeMap[key] ?? (value || '—');
}

/** 成员角色中文映射（UI 展示用；value 保持后端枚举） */
export const teamRoleMap: Record<string, string> = {
  worker: '执行',
  coordinator: '协调',
  synthesizer: '汇总',
  generator: '生成',
  critic: '评审',
};

export function teamRoleLabel(value?: string): string {
  const key = String(value || '')
    .trim()
    .toLowerCase();
  return teamRoleMap[key] ?? (value || '成员');
}

/** 失败策略取值中文映射 */
export const failurePolicyValueMap: Record<string, string> = {
  retry_then_block: '重试后阻塞',
  skip: '跳过',
  fail_fast: '快速失败',
  continue: '继续',
  abort: '中止',
  await_review: '暂停等审核',
  halt: '终止',
};

export function failurePolicyValueLabel(value?: string): string {
  const key = String(value || '')
    .trim()
    .toLowerCase();
  return failurePolicyValueMap[key] ?? (value || '—');
}

/** 运行 / 步骤状态中文映射（TeamRun / TeamRunStep 展示用） */
export function teamRunStatusLabel(status?: string): string {
  const key = String(status || '')
    .trim()
    .toLowerCase();
  if (key === 'success' || key === 'ok') return '已完成';
  if (key === 'error') return '失败';
  if (key === 'canceled') return '已取消';
  return teamStatusMap[key]?.label ?? (status || '—');
}

// ADR-08 A3：mode 下拉即模板选择器——description 展示在选项 caption，
// 选中后由派生链路（deriveMemberRolesForMode → rebuild/compile）重新生成 graph。
// UI 展示 5 项：adaptive 代表 Swarm（swarm 仍为合法 API 值，不单独出现在下拉以免重复「群智」）。
export const modeOptions = [
  { label: '顺序', value: 'sequential', description: '成员按顺序依次执行，上一步输出作为下一步上下文。' },
  { label: '并行', value: 'parallel', description: '成员并行产出，排序最后的启用成员负责汇总。' },
  { label: '主控', value: 'coordinator', description: '首位成员作为协调者分派任务，其余成员顺序执行。' },
  { label: '生成评审', value: 'critic_loop', description: '生成与评审角色交替迭代，直到达标或达迭代上限。' },
  {
    label: '群智',
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

/** 后端 team_state_machine.go 的前端镜像（recover/rework 转换已同步） */
export const validStatusTransitions: Record<string, string[]> = {
  pending: ['running', 'cancelled', 'failed'],
  running: ['completed', 'failed', 'cancelled', 'interrupted', 'pending'],
  interrupted: ['running'],
  completed: ['archived'],
  failed: ['archived', 'pending'],
  cancelled: ['archived', 'pending'],
  archived: [],
};

/** 判断状态转换是否合法 */
export function isValidStatusTransition(from: string, to: string): boolean {
  if (from === to) return true;
  return validStatusTransitions[from]?.includes(to) ?? false;
}

export const roleOptions = ['worker', 'coordinator', 'synthesizer', 'generator', 'critic'].map((value) => ({
  label: teamRoleLabel(value),
  value,
}));

export type TeamTemplateKey = 'sequential' | 'parallel_experts' | 'critic_loop' | 'coordinator';

export const teamTemplateOptions: Array<{ label: string; value: TeamTemplateKey; description: string }> = [
  { label: '顺序协作', value: 'sequential', description: '多个执行成员按顺序接力处理任务。' },
  {
    label: '并行专家组',
    value: 'parallel_experts',
    description: '前若干成员并行产出；列表中的最后一位 Agent 固定为汇总角色（与专家槽位不同实例）。',
  },
  {
    label: '生成评审',
    value: 'critic_loop',
    description: '「生成」与「评审」角色顺序迭代；迭代次数取自编排里的 critic_loop.max_iterations。',
  },
  {
    label: '主控分派',
    value: 'coordinator',
    description: '成员顺序执行；当前运行时为带迭代的上屏顺序链（非独立并行拓扑），适合分步接力。',
  },
];

export const failureDefaultOptions = ['retry_then_block', 'skip', 'fail_fast'].map((value) => ({
  label: failurePolicyValueLabel(value),
  value,
}));

export const parallelFailOptions = [
  { label: '继续（分支失败可跳过）', value: 'continue' },
  { label: '中止', value: 'abort' },
];

export const failureOnErrorOptions = ['await_review', 'halt'].map((value) => ({
  label: failurePolicyValueLabel(value),
  value,
}));

export const BuiltinIndustryId = '__builtin__';
export const PresetIndustryId = '__preset__';
