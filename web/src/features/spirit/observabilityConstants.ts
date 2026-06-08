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
