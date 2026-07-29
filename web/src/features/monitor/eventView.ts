/**
 * Monitor Events 归一化视图模型（方案：Events = 值得注意的事）。
 *
 * 纯函数模块：把 WS 运行时事件（TeamRunEvent）与持久化监控事件（monitor_events 行）
 * 映射为统一的 MonitorViewEvent —— 人话标题 / 一行摘要 / severity 分级 / 分类 / 主体。
 * i18n 通过注入 t() 完成（与 monitorTableUi 工厂模式一致），测试可注入回显假 t。
 */
import type { TeamRunEvent } from '../teams/types';
import type { MonitorEventsQuery, MonitorTrace, PlatformResource } from './types';
import {
  completionCanOpenInRuns,
  isRunnerCompletionRow,
  runnerCompletionMetaFromRow,
  type RunnerCompletionMeta,
} from './runCorrelation';
import { formatDate, parseJSON } from './utils';

/** vue-i18n t() 的最小签名（兼容带 named 参数调用） */
export type Translate = (key: string, ...args: unknown[]) => string;

export type MonitorEventSeverity = 'critical' | 'warn' | 'success' | 'info';

export type MonitorViewEvent = {
  id: string;
  /** 原始事件类型（详情/调试用，不直接展示） */
  type: string;
  /** 人话标题（i18n） */
  title: string;
  /** 一行摘要（hover/副行） */
  subtitle: string;
  /** 分类：task / message / agent / tool / system */
  category: string;
  severity: MonitorEventSeverity;
  /** 主体（Agent/规则/skill 名），无可为 '' */
  actor: string;
  time: string;
  /** 可打开会话的 session id */
  sessionId?: string;
  raw: unknown;
  completionMeta?: RunnerCompletionMeta;
  canOpenInRuns?: boolean;
  completionSessionId?: string;
};

/** 事件分类映射（需求 18-monitor.md §3.2；runner.completion 属 system 降级卡片） */
export function categoryForEventType(type: string): string {
  const t = String(type || '').trim();
  if (t === 'runner.completion' || t === 'runner_completion') return 'system';
  if (t === 'intent_pass') return 'agent';
  if (t.startsWith('run') || t.includes('team_run')) return 'task';
  if (t.startsWith('message') || t.startsWith('chat')) return 'message';
  if (t.startsWith('agent')) return 'agent';
  if (t.startsWith('tool') || t.includes('step')) return 'tool';
  return 'system';
}

/** 持久化事件 status → severity（status 来自写库方：info/warn/error/ok...） */
export function severityForPersistedStatus(status: string): MonitorEventSeverity {
  switch (
    String(status || '')
      .trim()
      .toLowerCase()
  ) {
    case 'critical':
    case 'error':
    case 'fatal':
      return 'critical';
    case 'warn':
    case 'warning':
      return 'warn';
    case 'ok':
    case 'success':
      return 'success';
    default:
      return 'info';
  }
}

/** WS 运行时事件 → severity（单次运行失败为 warn；完成 success；其余 info） */
export function severityForWsEvent(type: string, opts: { failed?: boolean } = {}): MonitorEventSeverity {
  const t = String(type || '');
  if (opts.failed) return 'warn';
  if (t.includes('failed') || t.includes('error')) return 'warn';
  if (t.includes('finished') || t.includes('completed')) return 'success';
  return 'info';
}

/** severity → Quasar 颜色名（表格色点用） */
export function severityColor(severity: MonitorEventSeverity): string {
  switch (severity) {
    case 'critical':
      return 'negative';
    case 'warn':
      return 'warning';
    case 'success':
      return 'positive';
    default:
      return 'info';
  }
}

/**
 * 历史表类型筛选选项 → ListMonitorEvents `event_type` 前缀。
 * 取值对齐真实落库 keyspace（RecordMonitorEvent 调用点）：
 * runner.completion / alert.* / skill.filesystem.* / usage.budget_alert / chat.user_feedback。
 */
export const EVENT_TYPE_FILTERS = [
  'all',
  'runner.completion',
  'alert.',
  'skill.filesystem.',
  'usage.budget_alert',
  'chat.user_feedback',
] as const;

/** severity 筛选 → monitor_events.status 精确值（写库方使用的取值集合） */
const SEVERITY_TO_STATUS: Record<string, string> = {
  critical: 'error',
  warn: 'warn',
  success: 'ok',
  info: 'info',
};

/** 历史查询组装（纯函数）：类型前缀 + 级别→status + 服务端分页 */
export function buildMonitorEventsQuery(opts: {
  type: string;
  severity: string;
  page: number;
  pageSize: number;
}): MonitorEventsQuery {
  const query: MonitorEventsQuery = {
    limit: opts.pageSize,
    offset: Math.max(0, (opts.page - 1) * opts.pageSize),
  };
  if (opts.type && opts.type !== 'all') query.event_type = opts.type;
  const status = SEVERITY_TO_STATUS[opts.severity];
  if (status) query.status = status;
  return query;
}

function shortSession(sessionId: string): string {
  const s = String(sessionId || '').trim();
  return s.length > 8 ? `${s.slice(0, 8)}…` : s;
}

/**
 * WS 运行时事件 → 视图模型。
 * @param seq 单调递增序号（替代旧实现中的数组长度，保证 id 稳定）
 */
export function wsEventToView(t: Translate, event: TeamRunEvent, seq: number): MonitorViewEvent {
  const type = event.type || 'runtime.event';
  const step = event.step;
  const run = event.run;
  const payload = event.payload || {};
  const sessionId =
    String(event.session_id || '').trim() ||
    String((payload as Record<string, unknown>).SessionID ?? '').trim() ||
    undefined;

  let title = type;
  let subtitle: string;
  const actor = String(step?.agent_name || '').trim();

  switch (type) {
    case 'team_run_started':
      title = t('monitorPage.events.title.teamRunStarted');
      subtitle = sessionId ? t('monitorPage.events.summary.session', { id: shortSession(sessionId) }) : '';
      break;
    case 'team_run_finished':
      title = t('monitorPage.events.title.teamRunFinished');
      subtitle = sessionId ? t('monitorPage.events.summary.session', { id: shortSession(sessionId) }) : '';
      break;
    case 'team_run_failed':
      title = t('monitorPage.events.title.teamRunFailed');
      subtitle = String(run?.error_message || '').trim();
      break;
    case 'team_step_started':
      title = t('monitorPage.events.title.stepStarted', { name: actor || '?' });
      subtitle = sessionId ? t('monitorPage.events.summary.session', { id: shortSession(sessionId) }) : '';
      break;
    case 'team_step_finished': {
      const failed = String(step?.status || '') === 'failed' || Boolean(step?.error_message);
      title = failed
        ? t('monitorPage.events.title.stepFailed', { name: actor || '?' })
        : t('monitorPage.events.title.stepFinished', { name: actor || '?' });
      subtitle = String(step?.error_message || '').trim();
      break;
    }
    case 'member_session_updated':
      title = t('monitorPage.events.title.memberUpdated', { name: actor || '?' });
      subtitle = String(step?.status || '').trim();
      break;
    case 'team_summary':
      title = t('monitorPage.events.title.teamSummary');
      subtitle = String(run?.output_preview || '').trim();
      break;
    case 'intent_pass': {
      title = t('monitorPage.events.title.intentPass');
      const outcome = String(payload.outcome ?? '');
      const ms = payload.duration_ms;
      subtitle =
        outcome +
        (typeof ms === 'number' ? ` · ${ms} ms` : '') +
        (payload.intent_kind ? ` · ${String(payload.intent_kind)}` : '');
      break;
    }
    default: {
      // 未知类型：回退原始 type，摘要取首个可用错误/输出预览，无占位废文案
      subtitle =
        String(step?.error_message || run?.error_message || '').trim() ||
        String(step?.output_preview || run?.output_preview || '').trim();
      break;
    }
  }

  const failed = type.includes('failed') || Boolean(step?.error_message || run?.error_message);
  return {
    id: `${type}-${run?.id || step?.id || 'e'}-${seq}`,
    type,
    title,
    subtitle,
    category: categoryForEventType(type),
    severity: severityForWsEvent(type, { failed }),
    actor,
    time: formatDate(step?.created_at || run?.updated_at || run?.created_at || new Date().toISOString()),
    sessionId,
    raw: event,
  };
}

/** 降级 completion 的人话标题（i18n 版，替代 runCorrelation 中的硬编码中文） */
function completionTitle(t: Translate, meta: RunnerCompletionMeta, rowName?: string): string {
  if (rowName?.trim()) return rowName.trim();
  return meta.status === 'error' ? t('monitorPage.events.title.chatFailed') : t('monitorPage.events.title.chatDone');
}

function completionSubtitle(t: Translate, meta: RunnerCompletionMeta, rowDesc?: string): string {
  if (rowDesc?.trim()) return rowDesc.trim();
  const parts: string[] = [];
  const agent = meta.agent_display_name || meta.agent_key;
  if (agent) parts.push(String(agent));
  if (meta.duration_ms && meta.duration_ms > 0) {
    parts.push(meta.duration_ms >= 1000 ? `${(meta.duration_ms / 1000).toFixed(1)} s` : `${meta.duration_ms} ms`);
  }
  const total = meta.usage?.total_tokens;
  if (total && total > 0) parts.push(t('monitorPage.events.summary.tokens', { count: total }));
  if (meta.session_id) parts.push(shortSession(meta.session_id));
  return parts.length ? parts.join(' · ') : t('monitorPage.events.summary.runDone');
}

/** 持久化监控事件（monitor_events 行）→ 视图模型 */
export function persistedEventToView(
  t: Translate,
  row: PlatformResource,
  traces: MonitorTrace[] = [],
): MonitorViewEvent {
  const cfg = parseJSON(row.config_json);
  const type = String(cfg.type || row.key || 'monitor.event');
  const completion = isRunnerCompletionRow(row);
  const meta = completion ? runnerCompletionMetaFromRow(row) : undefined;

  const severity =
    completion && meta?.status === 'error' && !String(row.status || '').trim()
      ? 'warn'
      : completion && meta?.status === 'error'
        ? 'warn'
        : severityForPersistedStatus(row.status);

  return {
    id: row.id,
    type,
    title: completion && meta ? completionTitle(t, meta, row.name) : row.name || type,
    subtitle:
      completion && meta
        ? completionSubtitle(t, meta, row.description)
        : // 摘要用 description；不回退 JSON.stringify(cfg)（原始 JSON 进详情弹窗）
          String(row.description || '').trim(),
    category: categoryForEventType(type),
    severity,
    actor: String(row.name || '').trim(),
    time: formatDate(row.created_at),
    sessionId: completion ? String(meta?.session_id || '').trim() || undefined : undefined,
    raw: { ...row, config: cfg, metadata: parseJSON(row.metadata_json) },
    completionMeta: meta,
    canOpenInRuns: completion && meta ? completionCanOpenInRuns(meta, traces) : false,
    completionSessionId: completion ? String(meta?.session_id || '').trim() || undefined : undefined,
  };
}
