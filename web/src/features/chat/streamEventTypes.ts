/**
 * Stream Event Types — 统一事件流类型系统
 *
 * 定义聊天事件流的 5 种 Activity Kind：
 * thinking / action / reply / error / plan
 *
 * 替代原有 activityTimelineTypes.ts 中的 Activity 类型，
 * 通过 re-export 保持向后兼容。
 *
 * 映射关系：
 *   ThinkActivity  → ThinkingEvent  (kind: 'think'   → 'thinking')
 *   ActActivity    → ActionEvent    (kind: 'act'     → 'action')
 *   SayActivity    → ReplyEvent     (kind: 'say'     → 'reply')
 *   NoticeActivity → ErrorEvent     (kind: 'notice'  → 'error')
 *   PlanEvent      → 新增           (kind: 'plan')
 *   DelegateActivity → 暂保留在 activityTimelineTypes
 */

// ── Stream Event Kind ──

/** 事件种类 — 时间线节点的语义分类 */
export type StreamEventKind = 'thinking' | 'action' | 'reply' | 'error' | 'plan' | 'confirm' | 'notice';

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

/** 错误/通知事件 — 对应原 NoticeActivity */
export interface ErrorEvent extends StreamEventBase {
  kind: 'error';
  type: 'degradation' | 'info';
  message: string;
}

/** 计划事件 — 新增类型 */
export interface PlanEvent extends StreamEventBase {
  kind: 'plan';
  steps: PlanStep[];
  status: 'planning' | 'executing' | 'completed' | 'failed';
  /** 计划标题 */
  title?: string;
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

// ── Stream Event Union ──

/** 统一事件流类型 */
export type StreamEvent = ThinkingEvent | ActionEvent | ReplyEvent | ErrorEvent | PlanEvent | ConfirmEvent | NoticeEvent;
