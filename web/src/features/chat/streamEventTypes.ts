/**
 * Stream Event Types — 统一事件流类型系统
 *
 * 定义聊天事件流的 7 种 Activity Kind：
 * thinking / action / reply / error / plan / confirm / notice
 *
 * 替代原有 activityTimelineTypes.ts 中的 Activity 类型，
 * 通过 re-export 保持向后兼容。
 */

// ── Stream Event Kind ──

/** 事件种类 — 时间线节点的语义分类 */
export type StreamEventKind =
  | 'thinking'
  | 'action'
  | 'reply'
  | 'error'
  | 'plan'
  | 'confirm'
  | 'notice'
  | 'team_stage'
  | 'graph_stage'
  | 'session';

/** 事件状态 */
export type StreamEventStatus = 'streaming' | 'completed' | 'failed';

// ── Stream Event Base ──

/** 事件基础字段 */
export interface StreamEventBase {
  id: string;
  kind: StreamEventKind;
}

// ── Plan Step ──

/** 计划步骤 */
export interface PlanStep {
  id: string;
  /** 步骤描述 — 对应后端 ActivityPlanStep.Label (JSON: "label") */
  label: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'partial_failure';
  agentKey: string | null;
  agentName: string | null;
  /** 依赖的步骤 ID 列表 */
  dependsOn?: string[];
  /** 步骤耗时（毫秒） */
  durationMs?: number | null;
  /** 子事件列表（递归 StreamEvent） */
  children?: StreamEvent[];
}

// ── Tool Activity ──

/** 工具活动（ActionEvent 的子结构） */
export interface ToolActivity {
  toolName: string;
  toolLabel: string;
  toolCategory?: string;
  status: 'running' | 'success' | 'failed' | 'blocked' | 'cancelled';
  durationMs: number | null;
  arguments: string | null;
  result: string | null;
  error: string | null;
  iconKey?: string;
  isLongRunning?: boolean;
}

// ── Individual Event Types ──

/** 思考事件 — 对应原 ThinkActivity */
export interface ThinkingEvent extends StreamEventBase {
  kind: 'thinking';
  content: string;
  /** 区分"规划"/"推理"/"重规划"/"进度" */
  label?: string;
  collapsed: boolean;
  streaming: boolean;
  durationMs: number | null;
  /** When multiple adjacent ThinkingEvents are merged, subSteps
   * contains the individual thinking steps. */
  subSteps?: ThinkingEvent[];
}

/** 动作事件 — 对应原 ActActivity */
export interface ActionEvent extends StreamEventBase {
  kind: 'action';
  tool: ToolActivity;
}

/** 回复事件 — 对应原 SayActivity */
export interface ReplyEvent extends StreamEventBase {
  kind: 'reply';
  content: string;
  /** isFinal 判定规则：Turn 内最后一条 assistant 消息的 reply 为 isFinal:true */
  isFinal: boolean;
  streaming: boolean;
  /** 渲染变体：默认 markdown / a2ui 结构化 UI */
  variant: 'default' | 'a2ui';
  /** A2UI 模式时的结构化数据 */
  a2uiLines?: Record<string, unknown>[];
  durationMs: number | null;
}

/** 错误事件 — 对应原 NoticeActivity */
export interface ErrorEvent extends StreamEventBase {
  kind: 'error';
  type: 'degradation';
  message: string;
  /**
   * Stable machine-readable error code used to drive inline action hints
   * (retry / switch_model / rephrase / …). May originate from:
   *   - `Activity.toolErrorCode` (turn-level codes like `LLM_CALL_FAILED`)
   *   - `Activity.meta.error_code` (backend `apierror.Code` like `NOT_FOUND`)
   *
   * See `features/chat/errorCodeHints.ts` for the full code → action map.
   */
  errorCode?: string;
}

/** 计划事件 — 新增类型 */
export interface PlanEvent extends StreamEventBase {
  kind: 'plan';
  steps: PlanStep[];
  status: 'planning' | 'executing' | 'completed' | 'failed';
  /** 计划标题 */
  title?: string;
  /** 子事件列表（从 activityTree 映射的递归 StreamEvent）。
   * B-04: 替代 PlanBlock 直接消费 activityTree 的数据流，统一通过事件模型传递。 */
  children?: StreamEvent[];
}

/** 确认事件 — 工具执行前的用户确认请求 */
export interface ConfirmEvent extends StreamEventBase {
  kind: 'confirm';
  /** 确认状态：tool_blocked=等待确认, completed=已批准, cancelled=已拒绝 */
  status: 'tool_blocked' | 'completed' | 'cancelled';
  /** 确认提示文本 */
  content: string;
  /** 工具名称 */
  toolName: string;
  /** 工具参数（JSON 字符串） */
  toolArguments: string | null;
  /** 自动批准时间（ISO 8601），存在时显示倒计时 */
  autoApproveAt?: string | null;
}

/** 通知事件 — 系统通知 */
export interface NoticeEvent extends StreamEventBase {
  kind: 'notice';
  type: 'info' | 'warning' | 'success';
  message: string;
}

// ── Stage Events (Phase 3: Team/Graph/Session unified rendering) ──

/** 团队成员状态 */
export interface TeamMemberStatus {
  agentKey: string;
  agentName: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  session_id?: string;
}

/** 团队阶段事件 — 团队组建/执行/完成 */
export interface TeamStageEvent extends StreamEventBase {
  kind: 'team_stage';
  /** 阶段状态 */
  status: 'running' | 'completed' | 'failed' | 'cancelled';
  /** 阶段描述（如"团队已组建"、"团队执行完成"） */
  title: string;
  /** 团队 ID */
  teamId?: string;
  /** 成员状态列表 */
  members?: TeamMemberStatus[];
  /** 任务摘要 */
  taskSummary?: string;
  /** 持续时间（毫秒） */
  durationMs?: number | null;
}

/** Graph DAG 节点状态 */
export interface GraphNodeStatus {
  nodeId: string;
  label: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped';
  dependsOn?: string[];
}

/** Graph 阶段事件 — DAG 执行进度 */
export interface GraphStageEvent extends StreamEventBase {
  kind: 'graph_stage';
  /** 阶段状态 */
  status: 'running' | 'completed' | 'failed' | 'cancelled';
  /** 阶段描述 */
  title: string;
  /** DAG 节点 ID */
  dagNodeId?: string;
  /** 节点状态列表 */
  nodes?: GraphNodeStatus[];
  /** 持续时间（毫秒） */
  durationMs?: number | null;
}

/** Session 阶段事件 — 子会话创建/执行 */
export interface SessionStageEvent extends StreamEventBase {
  kind: 'session';
  /** 阶段状态 */
  status: 'running' | 'completed' | 'failed' | 'cancelled';
  /** 会话描述（如"成员 A 正在执行"） */
  title: string;
  /** 子会话 ID */
  childSessionId?: string;
  /** 关联的 Agent key */
  agentKey?: string;
  /** 关联的 Agent 名称 */
  agentName?: string;
  /** 关联的团队 ID */
  teamId?: string;
  /** 父 Spirit Session ID */
  spiritSessionId?: string;
  /** 持续时间（毫秒） */
  durationMs?: number | null;
}

// ── Stream Event Union ──

/** 统一事件流类型 */
export type StreamEvent =
  | ThinkingEvent
  | ActionEvent
  | ReplyEvent
  | ErrorEvent
  | PlanEvent
  | ConfirmEvent
  | NoticeEvent
  | TeamStageEvent
  | GraphStageEvent
  | SessionStageEvent;
