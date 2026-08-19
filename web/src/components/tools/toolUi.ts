import type { QTableColumn } from 'quasar';
import { i18n } from '../../i18n';
import type { Tool, ToolInvocation, ToolInvocationAudit, ToolSummary } from '../../features/tools/types';
import type { ToolAgentBindingRow } from '../../features/tools/toolAgentBindingSummary';
import {
  REGISTRY_COL_W,
  registryCol,
  registryColActions,
  registryColEnabled,
} from '../../features/ui/registryTableColumns';

/** ToolsTable 列定义（函数形式，随 locale 取值） */
export function toolTableColumns(): QTableColumn<Tool>[] {
  const t = i18n.global.t;
  return [
    registryCol<Tool>('name', t('toolsPage.columns.tool'), 'display_name', 'left', REGISTRY_COL_W.nameWide),
    registryCol<Tool>('category', t('toolsPage.columns.categorySource'), 'category', 'left', REGISTRY_COL_W.category),
    registryCol<Tool>('runtime', t('toolsPage.columns.runtime'), 'runtime_status', 'left', REGISTRY_COL_W.status),
    registryColEnabled<Tool>(t('toolsPage.columns.enabled')),
    registryCol<Tool>('overrides', t('toolsPage.columns.overrides'), 'agent_override_count', 'center', REGISTRY_COL_W.narrow),
    registryCol<Tool>('stats', t('toolsPage.columns.stats'), 'invoke_count', 'left', REGISTRY_COL_W.status),
    registryCol<Tool>('success_rate', t('toolsPage.columns.successRate'), (row) => row.success_count, 'left', REGISTRY_COL_W.status),
    registryCol<Tool>('duration', t('toolsPage.columns.duration'), (row) => row.p95_duration_ms, 'left', REGISTRY_COL_W.status),
    registryCol<Tool>('last', t('toolsPage.columns.last'), 'last_invoked_at', 'left', REGISTRY_COL_W.timeWide),
    registryCol<Tool>('risk', t('toolsPage.columns.risk'), 'risk_level', 'left', REGISTRY_COL_W.status),
    registryColActions<Tool>(REGISTRY_COL_W.actions, t('toolsPage.columns.actions')),
  ];
}

/** ToolRunsTable 列定义（函数形式，随 locale 取值） */
export function toolRunsTableColumns(): QTableColumn<ToolInvocation>[] {
  const t = i18n.global.t;
  return [
    registryCol<ToolInvocation>('tool', t('toolsPage.columns.tool'), 'tool_key', 'left', REGISTRY_COL_W.nameWide),
    registryCol<ToolInvocation>('agent', t('toolsPage.columns.agent'), 'agent_id', 'left', REGISTRY_COL_W.agent),
    registryCol<ToolInvocation>('status', t('toolsPage.columns.status'), 'status', 'left', REGISTRY_COL_W.status),
    registryCol<ToolInvocation>('session_id', t('toolsPage.columns.session'), 'session_id', 'left', REGISTRY_COL_W.session),
    registryCol<ToolInvocation>('time', t('toolsPage.columns.timeDuration'), 'started_at', 'left', REGISTRY_COL_W.time),
    registryColActions<ToolInvocation>(REGISTRY_COL_W.narrow, t('toolsPage.columns.actions')),
  ];
}

/** ToolAuditsTable 列定义（函数形式，随 locale 取值） */
export function toolAuditsTableColumns(): QTableColumn<ToolInvocationAudit>[] {
  const t = i18n.global.t;
  return [
    registryCol<ToolInvocationAudit>('tool', t('toolsPage.columns.toolAction'), 'tool_key', 'left', REGISTRY_COL_W.name),
    registryCol<ToolInvocationAudit>('actor', t('toolsPage.columns.actor'), 'agent_id', 'left', REGISTRY_COL_W.agent),
    registryCol<ToolInvocationAudit>('status', t('toolsPage.columns.status'), 'status', 'left', REGISTRY_COL_W.status),
    registryCol<ToolInvocationAudit>('time', t('toolsPage.columns.time'), 'created_at', 'left', REGISTRY_COL_W.timeWide),
  ];
}

/** 列表筛选：分类（与常见 builtin/schema 对齐；值为英文枚举，无需 i18n） */
export const categoryFilterOptions = [
  'system',
  'web',
  'filesystem',
  'skill',
  'memory',
  'media',
  'runtime',
  'integration',
  'computeruse',
  'custom',
].map((value) => ({ label: value, value }));

/** 新建 Tool 时的来源提示（非枚举：仍为自由文本；函数形式随 locale 取值） */
export function sourceSuggestions(): { value: string; label: string; caption: string }[] {
  const t = i18n.global.t;
  return [
    { value: 'builtin', label: 'builtin', caption: t('toolsPage.options.sourceBuiltin') },
    { value: 'mcp', label: 'mcp', caption: t('toolsPage.options.sourceMcp') },
    { value: 'system', label: 'system', caption: t('toolsPage.options.sourceSystem') },
    { value: 'external', label: 'external', caption: t('toolsPage.options.sourceExternal') },
  ];
}

/** 列表「来源」筛选选项（与 sourceSuggestions 对齐；label 为英文枚举值） */
export function sourceFilterOptions(): { label: string; value: string }[] {
  return sourceSuggestions().map(({ value, label }) => ({ label, value }));
}

export function riskLevelOptions(): { label: string; value: string }[] {
  const t = i18n.global.t;
  return [
    { label: t('toolsPage.options.riskLow'), value: 'low' },
    { label: t('toolsPage.options.riskMedium'), value: 'medium' },
    { label: t('toolsPage.options.riskHigh'), value: 'high' },
    { label: t('toolsPage.options.riskCritical'), value: 'critical' },
  ];
}

/** 列表「启用」三态筛选 */
export function enabledTriStateOptions(): { label: string; value: boolean }[] {
  const t = i18n.global.t;
  return [
    { label: t('toolsPage.options.enabledOn'), value: true },
    { label: t('toolsPage.options.enabledOff'), value: false },
  ];
}

/** 调用记录状态（与后端 `tool_invocations.status` 对齐） */
export function toolInvocationStatusOptions(): { label: string; value: string }[] {
  const t = i18n.global.t;
  return [
    { label: t('toolsPage.invocationStatus.success'), value: 'success' },
    { label: t('toolsPage.invocationStatus.error'), value: 'error' },
    { label: t('toolsPage.invocationStatus.failed'), value: 'failed' },
    { label: t('toolsPage.invocationStatus.blocked'), value: 'blocked' },
    { label: t('toolsPage.invocationStatus.cancelled'), value: 'cancelled' },
  ];
}

const RISK_LABEL_KEYS: Record<string, string> = {
  low: 'riskLow',
  medium: 'riskMedium',
  high: 'riskHigh',
  critical: 'riskCritical',
};

/** Quasar 色名；语义对齐 UX §2 success / warning / danger，避免昼间霓虹 */
export function riskQuasarColor(value: string): string {
  const m: Record<string, string> = {
    low: 'positive',
    medium: 'warning',
    high: 'orange',
    critical: 'negative',
  };
  return m[value] ?? 'grey';
}

export function riskLabel(value: string): string {
  const key = RISK_LABEL_KEYS[value];
  return key ? i18n.global.t(`toolsPage.options.${key}`) : value;
}

export function runtimeStatusLabel(value?: string): string {
  const t = i18n.global.t;
  if (!value || value === 'available') return t('toolsPage.runtimeStatus.available');
  if (value === 'registered_only') return t('toolsPage.runtimeStatus.registeredOnly');
  if (value === 'disabled') return t('toolsPage.runtimeStatus.disabled');
  return value;
}

export function runtimeStatusColor(value?: string): string {
  if (value === 'disabled') return 'negative';
  if (value === 'registered_only') return 'grey';
  return 'positive';
}

export function runtimeKindHint(tool: Pick<Tool, 'supports_streaming'>): string {
  return tool.supports_streaming ? 'streaming' : 'function';
}

const STATUS_LABEL_KEYS: Record<string, string> = {
  success: 'success',
  error: 'error',
  blocked: 'blocked',
  cancelled: 'cancelled',
  failed: 'failed',
};

export function toolInvocationStatusLabel(value: string): string {
  const key = STATUS_LABEL_KEYS[value];
  return key ? i18n.global.t(`toolsPage.invocationStatus.${key}`) : value;
}

export function toolInvocationStatusColor(value: string): string {
  const m: Record<string, string> = {
    success: 'positive',
    error: 'negative',
    failed: 'negative',
    blocked: 'warning',
    cancelled: 'grey',
  };
  return m[value] ?? 'grey';
}

export function toolInvocationStatusIcon(value: string): string {
  const m: Record<string, string> = {
    success: 'check_circle',
    error: 'error',
    failed: 'error',
    blocked: 'block',
    cancelled: 'cancel',
  };
  return m[value] ?? 'help';
}

/** 汇总卡片数据（Tools 列表 API summary） */
export function buildToolSummaryCards(summary: ToolSummary) {
  const t = i18n.global.t;
  return [
    { label: t('toolsPage.summary.total'), value: String(summary.total_tools), hint: t('toolsPage.summary.totalHint') },
    { label: t('toolsPage.summary.enabled'), value: String(summary.enabled_tools), hint: t('toolsPage.summary.enabledHint') },
    { label: t('toolsPage.summary.highRisk'), value: String(summary.high_risk_enabled), hint: t('toolsPage.summary.highRiskHint') },
    {
      label: t('toolsPage.summary.calls24h'),
      value: String(summary.calls_24h),
      hint: t('toolsPage.summary.failureRateHint', { rate: formatFailureRatePercent(summary.failure_rate_24h) }),
    },
  ];
}

export function formatFailureRatePercent(rate: number): string {
  if (!Number.isFinite(rate)) return '—';
  return `${(rate * 100).toFixed(1)}%`;
}

/** 成功率（0~1）；无调用时返回 null。分母用总调用数（含 error/blocked/cancelled）。 */
export function toolSuccessRate(tool: Pick<Tool, 'invoke_count' | 'success_count'>): number | null {
  if (!tool.invoke_count || tool.invoke_count <= 0) return null;
  return tool.success_count / tool.invoke_count;
}

export function formatToolSuccessRate(tool: Pick<Tool, 'invoke_count' | 'success_count'>): string {
  const rate = toolSuccessRate(tool);
  if (rate == null) return '—';
  return `${(rate * 100).toFixed(1)}%`;
}

/** 成功率低于阈值（默认 90%）且有调用量时标红（需求 23-tools §5.2「成功率」列）。 */
export function toolSuccessRateColor(tool: Pick<Tool, 'invoke_count' | 'success_count'>, threshold = 0.9): string {
  const rate = toolSuccessRate(tool);
  if (rate == null) return 'grey';
  return rate < threshold ? 'negative' : 'positive';
}

/** 参数一次合法率（0~1）：1 − (repaired+invalid)/invoke，衡量模型对 schema 的一次性理解准确度；无调用返回 null。 */
export function toolArgsFirstPassRate(
  tool: Pick<Tool, 'invoke_count' | 'repaired_count' | 'invalid_count'>,
): number | null {
  if (!tool.invoke_count || tool.invoke_count <= 0) return null;
  const bad = tool.repaired_count + tool.invalid_count;
  if (bad <= 0) return 1;
  return Math.max(0, 1 - bad / tool.invoke_count);
}

export function formatToolArgsFirstPassRate(
  tool: Pick<Tool, 'invoke_count' | 'repaired_count' | 'invalid_count'>,
): string {
  const rate = toolArgsFirstPassRate(tool);
  if (rate == null) return '—';
  return `${(rate * 100).toFixed(1)}%`;
}

/** 一次合法率低于阈值（默认 95%）且有调用量时标黄——参数畸形会多耗一轮修复交互。 */
export function toolArgsFirstPassRateColor(
  tool: Pick<Tool, 'invoke_count' | 'repaired_count' | 'invalid_count'>,
  threshold = 0.95,
): string {
  const rate = toolArgsFirstPassRate(tool);
  if (rate == null) return 'grey';
  return rate < threshold ? 'warning' : 'positive';
}

export function prettyJSON(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw || '{}'), null, 2);
  } catch {
    return raw || '{}';
  }
}

/** 粗略 Schema 体量提示（字符长度 → 估算行复杂度） */
export function schemaSizeHint(json: string): string {
  const t = i18n.global.t;
  const len = (json || '').trim().length;
  if (len === 0) return t('toolsPage.schemaSize.empty');
  if (len < 120) return t('toolsPage.schemaSize.light');
  if (len < 2000) return t('toolsPage.schemaSize.medium');
  return t('toolsPage.schemaSize.large');
}

export function formatInvocationDuration(ms?: number): string {
  if (ms == null || Number.isNaN(ms)) return '—';
  // 1 位小数（去除尾随 .0），避免 3.8333333333333335ms 这类原始浮点直出
  if (ms < 1000) return `${Math.round(ms * 10) / 10}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

export function formatInvocationWhen(iso?: string): string {
  if (!iso) return '—';
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

/** 预览列截断，避免表格撑爆 */
export function clipPreview(text: string | undefined | null, max = 120): string {
  const t = (text ?? '').trim();
  if (t.length <= max) return t;
  return `${t.slice(0, max)}…`;
}

export type ToolMetricCard = { label: string; value: string; hint: string };

/** 从 Tool 行生成一行副标题（供详情/卡片） */
export function toolSubtitleLine(tool: Pick<Tool, 'key' | 'category' | 'source'>): string {
  return `${tool.key} · ${tool.category || 'custom'} · ${tool.source || '—'}`;
}

/** 编辑器 JSON 字段校验（与页面保存前一致） */
export function validateToolJsonFields(
  fields: Record<string, string>,
  keys: readonly string[],
): Record<string, string> {
  const errors: Record<string, string> = {};
  for (const key of keys) {
    try {
      JSON.parse(fields[key] || '{}');
    } catch (err) {
      errors[key] = err instanceof Error ? err.message : i18n.global.t('toolsPage.invalidJsonFallback');
    }
  }
  return errors;
}

export const toolEditorJsonKeys = [
  'parameters_schema_json',
  'result_schema_json',
  'config_schema_json',
  'config_json',
  'default_config_json',
  'metadata_json',
] as const;

export type ToolEditorJsonKey = (typeof toolEditorJsonKeys)[number];

/** Map JSON field → editor section id for validation focus. */
export function editorTabForJsonKey(key: string): 'params' | 'config' | 'advanced' {
  if (key === 'parameters_schema_json' || key === 'result_schema_json') {
    return 'params';
  }
  if (key === 'config_schema_json' || key === 'config_json') {
    return 'config';
  }
  return 'advanced';
}

/** First invalid JSON key in stable field order. */
export function firstInvalidToolJsonKey(errors: Record<string, string>): ToolEditorJsonKey | null {
  for (const key of toolEditorJsonKeys) {
    if (errors[key]) return key;
  }
  return null;
}

/**
 * 编辑器「变更预览」：当前 form 与打开时快照（originalForm）的浅比较。
 * 与 store dirty 判定同口径（原始值 !==），长值截断展示。
 */
export function diffToolFormLines(
  form: Record<string, unknown>,
  original: Record<string, unknown>,
): string[] {
  const lines: string[] = [];
  for (const key of Object.keys(form)) {
    const cur = form[key];
    const old = original[key];
    if (cur === old) continue;
    const fmt = (v: unknown) => clipPreview(typeof v === 'string' ? v : JSON.stringify(v), 48);
    lines.push(`${key}: ${fmt(old)} → ${fmt(cur)}`);
  }
  return lines;
}

export function invocationAgentLine(row: ToolInvocation): string {
  return row.agent_display_name || row.agent_key || row.agent_id || '—';
}

/** ToolDetailDrawer — Agent Binding 列定义（函数形式，随 locale 取值） */
export function agentBindingColumns(): QTableColumn<ToolAgentBindingRow>[] {
  const t = i18n.global.t;
  return [
    registryCol<ToolAgentBindingRow>('agent_name', t('toolsPage.agentBinding.colAgent'), 'agent_name', 'left', REGISTRY_COL_W.name),
    registryCol<ToolAgentBindingRow>('profile', t('toolsPage.agentBinding.colProfile'), 'profile', 'left', REGISTRY_COL_W.status),
    registryCol<ToolAgentBindingRow>('state', t('toolsPage.agentBinding.colState'), 'effective_state', 'left', REGISTRY_COL_W.status),
    registryCol<ToolAgentBindingRow>('reason', t('toolsPage.agentBinding.colReason'), 'reason', 'left', REGISTRY_COL_W.desc),
  ];
}

const TOOL_PROFILE_KEYS = [
  'chat_only',
  'read_only',
  'coding',
  'research',
  'full',
  'minimal',
  'safe',
  'system_admin',
  'spirit',
] as const;

export function toolProfileLabel(profile: string): string {
  const key = (profile || '').trim();
  if (!key) return '—';
  if ((TOOL_PROFILE_KEYS as readonly string[]).includes(key)) {
    return i18n.global.t(`toolsPage.profile.${key}`);
  }
  return key;
}

const BINDING_REASON_KEYS = [
  'agent_tools_disabled',
  'agent_deny',
  'global_disabled',
  'override_deny',
  'override_allow',
  'missing_api_key',
] as const;

/** 后端 reason → 中文。`profile:<name>` 动态映射为「策略：xx」。 */
export function bindingReasonLabel(reason: string): string {
  const r = (reason || '').trim();
  if (r.startsWith('profile:')) {
    return `${i18n.global.t('toolsPage.reason.profilePrefix')}${toolProfileLabel(r.slice('profile:'.length))}`;
  }
  if ((BINDING_REASON_KEYS as readonly string[]).includes(r)) {
    return i18n.global.t(`toolsPage.reason.${r}`);
  }
  return r || '—';
}

const OVERRIDE_MODE_KEYS = ['allow', 'deny', 'inherit'] as const;

export function overrideModeLabel(mode: string): string {
  const m = (mode || '').trim();
  if ((OVERRIDE_MODE_KEYS as readonly string[]).includes(m)) {
    return i18n.global.t(`toolsPage.overrideMode.${m}`);
  }
  return m;
}

const OVERRIDE_MODE_SHORT_KEYS: Record<string, string> = {
  allow: 'shortAllow',
  deny: 'shortDeny',
  inherit: 'shortInherit',
};

export function overrideModeShortLabel(mode: string): string {
  const m = (mode || '').trim();
  const key = OVERRIDE_MODE_SHORT_KEYS[m];
  return key ? i18n.global.t(`toolsPage.overrideMode.${key}`) : m;
}

/** 覆盖模式下拉选项（长标签），供 ToolOverrideEditorDialog / Agent 覆盖面板复用。 */
export function overrideModeOptions(): { label: string; value: string }[] {
  const t = i18n.global.t;
  return [
    { label: t('toolsPage.overrideMode.inheritLong'), value: 'inherit' },
    { label: t('toolsPage.overrideMode.allowLong'), value: 'allow' },
    { label: t('toolsPage.overrideMode.denyLong'), value: 'deny' },
  ];
}
