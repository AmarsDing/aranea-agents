/**
 * Observability constants for M59-OBS.
 *
 * Provides contextual loading message mappings and pulse configuration
 * used by useContextualLoadingMessage and useStatusPulse composables.
 */

// --- OBS-02: Contextual Loading Messages ---

export type ContextualLoadingConfig = {
  /** Event type pattern to match. */
  eventPattern: string;
  /** Quasar icon name for the loading indicator. */
  icon: string;
  /** Left border color (CSS color or Quasar color name). */
  color: string;
  /** Message template with {var} placeholders. */
  messageTemplate: string;
};

/**
 * Orchestration-phase contextual loading messages.
 * Displayed during the three-phase orchestration (plan → allocate → orchestrate).
 */
export const ORCHESTRATION_LOADING_MAP: ContextualLoadingConfig[] = [
  {
    eventPattern: 'butler.orchestration.started',
    icon: 'sync',
    color: 'grey',
    messageTemplate: '正在处理任务…',
  },
  {
    eventPattern: 'spirit_plan_created',
    icon: 'search',
    color: 'blue',
    messageTemplate: '正在分析任务复杂度…',
  },
  {
    eventPattern: 'spirit_allocation_created',
    icon: 'people',
    color: 'purple',
    messageTemplate: '正在分配 Agent 角色…',
  },
  {
    eventPattern: 'spirit_orchestration_started',
    icon: 'construction',
    color: 'orange',
    messageTemplate: '正在编排执行流程…',
  },
];

/**
 * Agent-level contextual loading messages.
 * Displayed during team execution (tool_call / tool_result).
 */
export const AGENT_LOADING_MAP: ContextualLoadingConfig[] = [
  {
    eventPattern: 'tool_call',
    icon: 'bolt',
    color: 'blue',
    messageTemplate: '{agentName} 正在{displayLabel}…',
  },
  {
    eventPattern: 'tool_result',
    icon: 'check_circle',
    color: 'green',
    messageTemplate: '{agentName} 完成，耗时 {durationSec}s',
  },
];

// --- P-ORCH.2: Orchestration Progress Phases ---

/**
 * Fine-grained orchestration progress phases emitted by the backend
 * (TaskPlanner / AgentAllocator / AgentFactory) as SystemNoticeEvent with
 * NoticeType="orchestration_progress". The frontend receives these as
 * ActivityEvent(kind=notice, stage=orchestration_progress) with meta.phase
 * selecting the rendering template.
 *
 * `messageKey` is an i18n key under `chat.orchestrationProgress.*`. The
 * composable translates it via `t()` and substitutes placeholders from
 * `activity.meta`:
 *   - {sub_task_count}  (decomposed)
 *   - {index}           (allocating)
 *   - {total}           (allocating, allocated)
 *   - {sub_task}        (allocating)
 *   - {agent_name}      (creating_agent, agent_created)
 *
 * Phases align with the backend table in docs/development/1-chat.design.md
 * (P-ORCH.1). Loading messages replace (not append) the previous one so the
 * user sees live progress.
 */
export type OrchestrationProgressConfig = {
  /** i18n key under `chat.orchestrationProgress.*`. */
  messageKey: string;
  /** Quasar icon name for the loading indicator. */
  icon: string;
  /** Left border color (CSS color or Quasar color name). */
  color: string;
};

export const ORCHESTRATION_PROGRESS_MAP: Record<string, OrchestrationProgressConfig> = {
  // Pre-orchestration turn phases (2026-08-06): emitted by ChatOrchestrator
  // (publishTurnProgress) between message ack and planning, closing the
  // previously silent window (recall / MCP tools build / IntentPass / gate).
  routing: {
    messageKey: 'chat.orchestrationProgress.routing',
    icon: 'route',
    color: 'grey',
  },
  recalling: {
    messageKey: 'chat.orchestrationProgress.recalling',
    icon: 'psychology',
    color: 'teal',
  },
  preparing_tools: {
    messageKey: 'chat.orchestrationProgress.preparingTools',
    icon: 'build',
    color: 'blue-grey',
  },
  understanding: {
    messageKey: 'chat.orchestrationProgress.understanding',
    icon: 'lightbulb',
    color: 'blue',
  },
  assessing: {
    messageKey: 'chat.orchestrationProgress.assessing',
    icon: 'analytics',
    color: 'indigo',
  },
  starting: {
    messageKey: 'chat.orchestrationProgress.starting',
    icon: 'play_circle',
    color: 'orange',
  },
  decomposing: {
    messageKey: 'chat.orchestrationProgress.decomposing',
    icon: 'split',
    color: 'blue',
  },
  decomposed: {
    messageKey: 'chat.orchestrationProgress.decomposed',
    icon: 'check_circle',
    color: 'blue',
  },
  // 2026-08-07 00:52 会话修复：分解失败/产空时的显式降级通知（reason=error|empty）。
  decompose_failed: {
    messageKey: 'chat.orchestrationProgress.decomposeFailed',
    icon: 'warning',
    color: 'orange',
  },
  // P3（2026-08-08）：分解 LLM 瞬时故障重试中（meta.attempt 为即将开始的尝试序号）。
  decompose_retry: {
    messageKey: 'chat.orchestrationProgress.decomposeRetry',
    icon: 'refresh',
    color: 'orange',
  },
  allocating: {
    messageKey: 'chat.orchestrationProgress.allocating',
    icon: 'people',
    color: 'purple',
  },
  allocated: {
    messageKey: 'chat.orchestrationProgress.allocated',
    icon: 'check_circle',
    color: 'purple',
  },
  creating_agent: {
    messageKey: 'chat.orchestrationProgress.creatingAgent',
    icon: 'add_circle',
    color: 'orange',
  },
  agent_created: {
    messageKey: 'chat.orchestrationProgress.agentCreated',
    icon: 'check_circle',
    color: 'green',
  },
  // 2026-08-21 P0：teamCount 截断/放行显式通知（task_planner_impl.go）。
  // composable 按 meta.action 选择 truncate / proceed 文案。
  team_count_mismatch: {
    messageKey: 'chat.orchestrationProgress.teamCountMismatch',
    icon: 'warning',
    color: 'orange',
  },
};

// --- OBS-05: Sidebar Status Pulse ---

export type PulseConfig = {
  /** CSS color for the pulse animation. */
  color: string;
  /** Duration of the pulse animation in milliseconds. */
  durationMs: number;
};

/**
 * Pulse CSS color and duration by team status transition.
 * Key is the NEW status value (SpiritTeamStatus).
 * Colors are CSS variable references for theme consistency.
 */
export const PULSE_COLOR_MAP: Record<string, string> = {
  running: 'var(--color-accent)',
  completed: 'var(--color-success)',
  failed: 'var(--color-danger)',
  interrupted: 'var(--color-warning)',
};

export const PULSE_DURATION_MAP: Record<string, number> = {
  running: 1000,
  completed: 1500,
  failed: 2000,
  interrupted: 1500,
};

// --- OBS-01: Auto-Collapse Summary Templates ---

/** Default max parallel teams quota. Shared by ChatPage and TaskExecutionPanel. */
export const DEFAULT_MAX_PARALLEL_TEAMS = 3;

export type BlockSummaryConfig = {
  /** Icon for the collapsed summary line. */
  icon: string;
  /** Template with {toolName}, {count}, {durationSec}, {teamName}, {completedSteps}, {totalSteps} placeholders. */
  template: string;
};

export const TOOL_BLOCK_SUMMARY: BlockSummaryConfig = {
  icon: 'build',
  template: '{count} tools · {durationSec}s',
};

export const TEAM_ASSEMBLY_SUMMARY: BlockSummaryConfig = {
  icon: 'groups',
  template: '组建团队 → {teamName}',
};

export const TEAM_COMPLETED_SUMMARY: BlockSummaryConfig = {
  icon: 'check_circle',
  template: '{teamName} · {durationSec}s',
};

export const TEAM_INTERRUPTED_SUMMARY: BlockSummaryConfig = {
  icon: 'pause_circle',
  template: '⏸ {teamName} · {completedSteps}/{totalSteps} 步骤',
};
