/**
 * Activity Timeline Types — 聊天活动时间线 UI 组合类型
 *
 * 基于"活动时间线"模型重设计，替代原有的"消息列表"模型。
 * 每个用户 Turn 内的所有内容按时间顺序排列为活动时间线。
 *
 * 底层事件类型（StreamEvent 等）定义在 streamEventTypes.ts，
 * 本文件仅定义高层 UI 组合类型（ConversationTurn、TeamPanel 等）。
 * 消费者应直接从 streamEventTypes.ts 导入事件类型。
 *
 * 设计文档：docs/reports/2026-06-12-proposal-chat-activity-timeline-redesign.md
 */

import type { Message } from './types';
import type { OrchestrationPlan, PlanEntry, TeamStatusSummary, ProgressSection } from './agentTreeTypes';
import type { StreamEvent } from './streamEventTypes';
import type { ActivityTreeNode } from './activityTypes';

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

  /** Activity tree for resolving parent-child relationships (plan sub-events) */
  activityTree?: ActivityTreeNode[];

  /** Team 统一面板（Team 模式时存在） */
  panel?: TeamPanel;

  /** 以下字段从现有 AgentBlock 迁移，保留语义 */
  task: string | null;
  result: string | null;
  hasPartialFailure: boolean;
  plan: OrchestrationPlan | null;
  teamStatus: TeamStatusSummary | null;
  progressSections: ProgressSection[];
  startedAt: string;
  finishedAt: string | null;
}

// ── Activity (时间线节点) ──

/**
 * 活动节点 — 时间线上的最小展示单元。
 * N-08: DelegateActivity 类型已移除，委托活动通过 StreamEvent 体系表达。
 */
export type Activity = StreamEvent;

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
  entries: Array<
    PlanEntry & {
      num: number;
      agentName: string | null;
    }
  >;
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
