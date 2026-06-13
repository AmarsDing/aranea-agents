/**
 * Activity Timeline Types — 聊天活动时间线数据模型
 *
 * 基于"活动时间线"模型重设计，替代原有的"消息列表"模型。
 * 每个用户 Turn 内的所有内容按时间顺序排列为活动时间线。
 *
 * 设计文档：docs/reports/2026-06-12-proposal-chat-activity-timeline-redesign.md
 */

import type { Message } from './types';
import type {
  OrchestrationPlan,
  PlanEntry,
  TeamStatusSummary,
  ProgressSection,
  ToolSectionStatus,
  TaskBoardNodeData,
} from './agentTreeTypes';

// ── Conversation Turn ──

/** 一个 Turn = 用户提问 + Agent 工作过程 */
export interface ConversationTurn {
  /** 使用后端 turn_id（权威 FK），不用前端推算 */
  id: string;
  userMessage: Message | null;
  agentWork: AgentWorkProcess;
}

// ── Agent Work Process ──

/** Agent 工作过程 = 活动时间线 */
export interface AgentWorkProcess {
  agentKey: string;
  agentName: string;
  agentIcon: string;
  agentColor: string;
  /** 扩展自 AgentBlockStatus，合并 tool_running/tool_blocked 到 running，partial_failure 映射为 completed */
  status: 'running' | 'completed' | 'failed';
  durationMs: number | null;

  /** 活动时间线 — 严格按发生顺序排列 */
  activities: Activity[];

  /** Team 统一面板（Team 模式时存在） */
  panel?: TeamPanel;

  /** TaskBoard 树状节点（Activity-First 模式，有数据时优先渲染 TaskBoard） */
  taskBoardNodes?: TaskBoardNodeData[];

  /** 以下字段从现有 AgentBlock 迁移，保留语义 */
  task: string | null;
  result: string | null;
  hasPartialFailure: boolean;
  plan: OrchestrationPlan | null;
  teamStatus: TeamStatusSummary | null;
  progressSections: ProgressSection[];
  startedAt: string;
  finishedAt: string | null;
  /** AF-Phase3: true when this turn was built without Activity data (pre-AF session).
   * The UI should render a simplified view instead of the full activity timeline. */
  isLegacy?: boolean;
}

// ── Activity (时间线节点) ──

/** 活动节点 — 时间线上的最小展示单元 */
export type Activity =
  | ThinkActivity
  | ActActivity
  | SayActivity
  | DelegateActivity
  | NoticeActivity;

export interface ThinkActivity {
  kind: 'think';
  id: string;
  content: string;
  /** 区分"规划"/"推理"/"重规划"/"进度" */
  label?: string;
  collapsed: boolean;
  streaming: boolean;
  durationMs: number | null;
  /** D3: When multiple adjacent ThinkActivities are merged, subSteps
   * contains the individual thinking steps. When undefined, this is
   * a single (unmerged) ThinkActivity. */
  subSteps?: ThinkActivity[];
}

export interface ActActivity {
  kind: 'act';
  id: string;
  tool: ToolActivity;
}

export interface SayActivity {
  kind: 'say';
  id: string;
  content: string;
  /** isFinal 判定规则：Turn 内最后一条 assistant 消息的 say 为 isFinal:true */
  isFinal: boolean;
  streaming: boolean;
  /** 渲染变体：默认 markdown / a2ui 结构化 UI */
  variant: 'default' | 'a2ui';
  /** A2UI 模式时的结构化数据 */
  a2uiLines?: Record<string, unknown>[];
  durationMs: number | null;
}

export interface DelegateActivity {
  kind: 'delegate';
  id: string;
  subAgent: AgentWorkProcess;
}

/** 通知/提示节点 — 对应现有 TimelineEntry.kind === 'notice' */
export interface NoticeActivity {
  kind: 'notice';
  id: string;
  type: 'degradation' | 'info';
  message: string;
}

// ── Tool Activity ──

/** 工具活动 */
export interface ToolActivity {
  toolName: string;
  toolLabel: string;
  status: ToolSectionStatus;
  durationMs: number | null;
  arguments: string | null;
  result: string | null;
  error: string | null;
  iconKey?: string;
  isLongRunning?: boolean;
}

// ── Team Panel ──

/** Team 统一面板数据 */
export interface TeamPanel {
  /** 任务拆解 — 复用现有 PlanEntry 类型 */
  taskBoard: TaskBoardSection;
  /** 依赖关系 DAG */
  dag?: DagSection;
  /** 团队进度 */
  teamProgress: TeamProgressSection[];
}

/** 任务拆解 — 扩展现有 PlanEntry */
export interface TaskBoardSection {
  /** 复用现有 OrchestrationPlan 的 PlanEntry，增加序号和 Agent 分配 */
  entries: Array<PlanEntry & {
    num: number;
    agentName: string | null;
  }>;
}

/** DAG 依赖关系 */
export interface DagSection {
  nodes: Array<{
    id: string;
    label: string;
    status: 'done' | 'running' | 'pending' | 'failed';
  }>;
  edges: Array<{ from: string; to: string }>;
}

/** 团队进度 */
export interface TeamProgressSection {
  teamId: string;
  teamName: string;
  teamIcon: string;
  status: 'running' | 'completed' | 'failed' | 'interrupted';
  progressPercent: number;
  durationMs: number | null;
  /** Agent 详情 — 复用 Activity 模型 */
  agents: AgentProgress[];
  /** 中断时的操作 */
  actions?: ('resume' | 'cancel')[];
}

/** Agent 进度 — 复用 Activity 而非独立 Line 类型 */
export interface AgentProgress {
  agentKey: string;
  agentName: string;
  agentIcon: string;
  status: 'running' | 'completed' | 'failed' | 'waiting';
  /** 复用 Activity 类型，通过 variant 控制紧凑/展开渲染 */
  activities: Activity[];
}

// ── Activity Variant ──

/** Activity 渲染变体：card（折叠卡片）或 compact（紧凑行） */
export type ActivityVariant = 'card' | 'compact';
