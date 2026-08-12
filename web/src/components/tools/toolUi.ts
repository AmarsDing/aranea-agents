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

/** ToolsTable 列定义 */
export const TOOL_TABLE_COLUMNS: QTableColumn<Tool>[] = [
  registryCol<Tool>('name', 'Tool', 'display_name', 'left', REGISTRY_COL_W.nameWide),
  registryCol<Tool>('category', '分类 / 来源', 'category', 'left', REGISTRY_COL_W.category),
  registryCol<Tool>('runtime', '运行时', 'runtime_status', 'left', REGISTRY_COL_W.status),
  registryColEnabled<Tool>(),
  registryCol<Tool>('overrides', '覆盖', 'agent_override_count', 'center', REGISTRY_COL_W.narrow),
  registryCol<Tool>('stats', '使用频率', 'invoke_count', 'left', REGISTRY_COL_W.status),
  registryCol<Tool>('success_rate', '成功率', (row) => row.success_count, 'left', REGISTRY_COL_W.status),
  registryCol<Tool>('duration', '耗时', (row) => row.p95_duration_ms, 'left', REGISTRY_COL_W.status),
  registryCol<Tool>('last', '最近调用', 'last_invoked_at', 'left', REGISTRY_COL_W.timeWide),
  registryCol<Tool>('risk', '风险', 'risk_level', 'left', REGISTRY_COL_W.status),
  registryColActions<Tool>(),
];

/** ToolRunsTable 列定义 */
export const TOOL_RUNS_TABLE_COLUMNS: QTableColumn<ToolInvocation>[] = [
  registryCol<ToolInvocation>('tool', 'Tool', 'tool_key', 'left', REGISTRY_COL_W.nameWide),
  registryCol<ToolInvocation>('agent', 'Agent', 'agent_id', 'left', REGISTRY_COL_W.agent),
  registryCol<ToolInvocation>('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
  registryCol<ToolInvocation>('session_id', 'Session', 'session_id', 'left', REGISTRY_COL_W.session),
  registryCol<ToolInvocation>('time', '时间 / 耗时', 'started_at', 'left', REGISTRY_COL_W.time),
  registryColActions<ToolInvocation>(REGISTRY_COL_W.narrow, '操作'),
];

/** ToolAuditsTable 列定义 */
export const TOOL_AUDITS_TABLE_COLUMNS: QTableColumn<ToolInvocationAudit>[] = [
  registryCol<ToolInvocationAudit>('tool', 'Tool / Action', 'tool_key', 'left', REGISTRY_COL_W.name),
  registryCol<ToolInvocationAudit>('actor', 'Agent / User', 'agent_id', 'left', REGISTRY_COL_W.agent),
  registryCol<ToolInvocationAudit>('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
  registryCol<ToolInvocationAudit>('time', '时间', 'created_at', 'left', REGISTRY_COL_W.timeWide),
];

/** 列表筛选：分类（与常见 builtin/schema 对齐） */
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

/** 新建 Tool 时的来源提示（非枚举：仍为自由文本） */
export const sourceSuggestions = [
  { value: 'builtin', label: 'builtin', caption: '平台内置' },
  { value: 'mcp', label: 'mcp', caption: 'MCP 暴露' },
  { value: 'system', label: 'system', caption: '系统级' },
  { value: 'external', label: 'external', caption: '外部注册' },
];

/** 列表「来源」筛选选项（与 sourceSuggestions 对齐） */
export const sourceFilterOptions = sourceSuggestions.map(({ value, label }) => ({ label, value }));

export const riskLevelOptions = [
  { label: '低', value: 'low' },
  { label: '中', value: 'medium' },
  { label: '高', value: 'high' },
  { label: '严重', value: 'critical' },
];

/** 列表「启用」三态筛选 */
export const enabledTriStateOptions = [
  { label: '已启用', value: true },
  { label: '已禁用', value: false },
];

/** 调用记录状态（与后端 `tool_invocations.status` 对齐） */
export const toolInvocationStatusOptions = [
  { label: '成功', value: 'success' },
  { label: '错误', value: 'error' },
  { label: '阻断', value: 'blocked' },
  { label: '取消', value: 'cancelled' },
];

const RISK_LABELS: Record<string, string> = {
  low: '低',
  medium: '中',
  high: '高',
  critical: '严重',
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
  return RISK_LABELS[value] ?? value;
}

export function runtimeStatusLabel(value?: string): string {
  if (!value || value === 'available') return '可用';
  if (value === 'registered_only') return '仅注册';
  if (value === 'disabled') return '禁用';
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

const STATUS_LABELS: Record<string, string> = {
  success: '成功',
  error: '错误',
  blocked: '阻断',
  cancelled: '取消',
  failed: '失败',
};

export function toolInvocationStatusLabel(value: string): string {
  return STATUS_LABELS[value] ?? value;
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

/** 汇总卡片数据（Tools 列表 API summary） */
export function buildToolSummaryCards(summary: ToolSummary) {
  return [
    { label: '总工具', value: String(summary.total_tools), hint: '已注册 Tool' },
    { label: '已启用', value: String(summary.enabled_tools), hint: '全局启用' },
    { label: '高风险启用', value: String(summary.high_risk_enabled), hint: 'high / critical' },
    {
      label: '24h 调用',
      value: String(summary.calls_24h),
      hint: `失败率 ${formatFailureRatePercent(summary.failure_rate_24h)}`,
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

export function prettyJSON(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw || '{}'), null, 2);
  } catch {
    return raw || '{}';
  }
}

/** 粗略 Schema 体量提示（字符长度 → 估算行复杂度） */
export function schemaSizeHint(json: string): string {
  const len = (json || '').trim().length;
  if (len === 0) return '空';
  if (len < 120) return '轻量';
  if (len < 2000) return '中等';
  return '较大';
}

export function formatInvocationDuration(ms?: number): string {
  if (ms == null || Number.isNaN(ms)) return '—';
  if (ms < 1000) return `${ms}ms`;
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
      errors[key] = err instanceof Error ? err.message : 'JSON 格式错误';
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
