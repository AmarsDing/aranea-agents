/**
 * 作战总台（Workbench Cockpit）共享类型契约。
 * 展示组件只允许从这里 import 类型（数据流铁律：组件不碰 api/store）。
 */

// ——— YOU 金门四 lane ———
export type YouLaneId = 'trade' | 'publish' | 'listing' | 'alerts';

/** 行内可执行动作：approve/reject/ack = 写路径；goto = 只下钻导航（无写路径的 lane） */
export type YouActionKind = 'approve' | 'reject' | 'ack' | 'goto';

export type YouItem = {
  id: string;
  lane: YouLaneId;
  /** 归因专项 agent_key；无 join key 时为空串（诚实口径，不画假归因） */
  agentKey: string;
  who: string;
  /** ISO 时间戳；无证据时为空串，展示层渲染 — */
  when: string;
  summary: string;
  action: YouActionKind;
  /** goto 动作的路由目标 */
  to?: string;
  /** 资金/高危类：移动端二次确认 */
  danger?: boolean;
};

export type YouLaneState = {
  lane: YouLaneId;
  items: YouItem[];
  count: number;
  /** 该 lane 最近一次成功刷新时间（ISO），信任栏新鲜度来源 */
  updatedAt: string;
  /** 该 lane 任一子源拉取失败 → 降级注记（不打正常高分） */
  degraded: boolean;
};

/** 签收回执（真实写路径成功后产生；驱动落款戳 + 日志绿动词行） */
export type SignReceipt = {
  itemId: string;
  lane: YouLaneId;
  verb: 'approved' | 'rejected' | 'acked';
  title: string;
  at: string; // ISO
};

// ——— 花名册 / 角色行 ———
export type LiveKind = 'idle' | 'thinking' | 'working' | 'waiting' | 'error';

export type ToolBeat = {
  toolName: string;
  durationMs: number;
  errorCode: string;
  at: string; // ISO
};

export type AgentLiveState = {
  agentKey: string;
  statusKind: LiveKind;
  /** 状态词（原始文案；展示层只读，i18n 由组件按 statusKind 兜底） */
  statusWord: string;
  lastToolName: string;
  lastDurationMs: number;
  lastErrorCode: string;
  /** 最近事件时间（ISO）；超出点亮窗口（3s）模拟为静止 */
  lastEventAt: string;
  /** 最近工具心跳（抽屉 L2 工具面 + ACTIVITY 卡 ticker） */
  recentTools: ToolBeat[];
  /** 最近一次思考流截断（≤120 字；抽屉 L1；诚实口径，无数据空串） */
  lastThinking: string;
};

export type RosterAgent = {
  key: string;
  name: string;
  icon: string;
  positionId: string;
  positionName: string;
  deptId: string;
  deptName: string;
  bizLine: BizLineId | '';
};

export type RosterPosition = {
  id: string;
  name: string;
  agents: RosterAgent[];
};

export type RosterDept = {
  id: string;
  name: string;
  bizLine: BizLineId | '';
  positions: RosterPosition[];
  /** 挂在部门下但未绑岗位的专项 */
  floatAgents: RosterAgent[];
};

export type RosterModel = {
  depts: RosterDept[];
  /** 完全无组织归属的专项（降级分组） */
  unassigned: RosterAgent[];
};

// ——— 业务线（四线卡网格 / SOURCE 权限卡 / 走廊 ×N 合并键）———
export type BizLineId = 'novatrader' | 'hotcraft' | 'assistant' | 'ecmove';

export type BizLineCard = {
  id: BizLineId;
  /** 有后端证据= true；false 整卡暗态显待接入（验收红线 1） */
  connected: boolean;
  /** 证据摘要行（计数文本由组件按 i18n 渲染，这里只给数值） */
  pending: number;
  done: number;
  note: string;
};

// ——— 事件日志卷宗 ———
export type LogKind = 'agent' | 'you' | 'error' | 'gold';

export type LogLine = {
  id: string;
  at: string; // ISO
  kind: LogKind;
  agentKey: string;
  text: string;
};

// ——— 信任六态（R1/R5 同源）———
export type TrustState = 'live' | 'offline' | 'stale' | 'degraded' | 'backlog' | 'replay';

// ——— 走廊 ———
export type CorridorPoint = { x: number; y: number };

export type CorridorLink = {
  /** 稳定 id（agent key 或 bizline 合并键） */
  id: string;
  agentKey: string;
  bizLine: BizLineId | '';
  from: CorridorPoint;
  to: CorridorPoint;
  active: boolean;
  awaiting: boolean;
  /** 合并载波计数；>1 渲染 ×N */
  merged: number;
};

// ——— 下钻抽屉 ———
export type DrillCard = 'dot' | 'job' | 'rule' | 'activity';

export type DrillTarget = {
  agentKey: string;
  agentName: string;
  card: DrillCard;
} | null;

// ——— 心跳（今日已完成）———
export type CompletionKind = 'decision' | 'task' | 'team_run' | 'seal';

export type CompletionBeat = {
  kind: CompletionKind;
  at: string;
  label: string;
};
