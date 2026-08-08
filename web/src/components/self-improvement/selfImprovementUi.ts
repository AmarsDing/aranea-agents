import type { QTableColumn } from 'quasar';
import type { SIRun } from '../../features/self-improvement/types';
import { registryCol, registryColActions } from '../../features/ui/registryTableColumns';

/** vue-i18n t() 的最小签名（兼容带 named 参数调用） */
type Translate = (key: string, ...args: unknown[]) => string;

/** Runs 列表列定义（73-self-iteration-v3 design §八：状态/风险/触发源/类型/变更/尝试/时间/操作） */
export function createSIRunColumns(t: Translate): QTableColumn<SIRun>[] {
  return [
    registryCol<SIRun>('id', 'ID', 'id', 'left', '16%'),
    registryCol<SIRun>('status', t('selfImprovementPage.colStatus'), 'status', 'center', '10%'),
    registryCol<SIRun>('riskLevel', t('selfImprovementPage.colRisk'), 'riskLevel', 'center', '8%'),
    registryCol<SIRun>('triggerSource', t('selfImprovementPage.colTrigger'), 'triggerSource', 'left', '12%'),
    registryCol<SIRun>('patchKind', t('selfImprovementPage.colKind'), 'patchKind', 'center', '8%'),
    registryCol<SIRun>('diffStats', t('selfImprovementPage.colDiff'), 'diffStats', 'left', '12%'),
    registryCol<SIRun>('attempts', t('selfImprovementPage.colAttempts'), 'attempts', 'center', '6%'),
    registryCol<SIRun>('createdAt', t('selfImprovementPage.colCreated'), 'createdAt', 'left', '18%'),
    registryColActions<SIRun>('10%', ''),
  ];
}

// ── 状态（D3 状态机） ────────────────────────────────────────────────────────

export function siStatusLabel(t: Translate, status: string): string {
  const key = `selfImprovementPage.status.${status || 'unknown'}`;
  const label = t(key);
  return label === key ? status || '—' : label;
}

export function siStatusColor(status: string): string {
  switch (status) {
    case 'closed':
      return 'positive';
    case 'rolled_back':
    case 'verify_failed':
    case 'rejected':
    case 'failed':
      return 'negative';
    case 'awaiting_governance':
      return 'warning';
    case 'applied':
    case 'observing':
      return 'info';
    case 'applying':
      return 'secondary';
    default:
      return 'grey-7';
  }
}

// ── 风险等级（D6） ───────────────────────────────────────────────────────────

export function siRiskLabel(t: Translate, risk: string): string {
  const key = `selfImprovementPage.risk.${risk || 'unknown'}`;
  const label = t(key);
  return label === key ? risk || '—' : label;
}

export function siRiskColor(risk: string): string {
  switch (risk) {
    case 'low':
      return 'positive';
    case 'medium':
      return 'warning';
    case 'high':
      return 'negative';
    default:
      return 'grey-7';
  }
}

// ── 触发源（D2） ─────────────────────────────────────────────────────────────

export function siTriggerLabel(t: Translate, trigger: string): string {
  const key = `selfImprovementPage.trigger.${trigger || 'unknown'}`;
  const label = t(key);
  return label === key ? trigger || '—' : label;
}

export function siTriggerColor(trigger: string): string {
  switch (trigger) {
    case 'error_cluster':
      return 'deep-orange';
    case 'perf_bottleneck':
      return 'indigo';
    case 'eval_regression':
      return 'purple';
    case 'test_failure':
      return 'teal';
    default:
      return 'grey-7';
  }
}

// ── 补丁类型（D6 R1 soft kinds） ─────────────────────────────────────────────

export function siKindLabel(t: Translate, kind: string): string {
  const key = `selfImprovementPage.kind.${kind || 'unknown'}`;
  const label = t(key);
  return label === key ? kind || '—' : label;
}

// ── 治理通道（D6 channel 映射） ──────────────────────────────────────────────

export function siChannelLabel(t: Translate, channel: string): string {
  const key = `selfImprovementPage.channel.${channel || 'unknown'}`;
  const label = t(key);
  return label === key ? channel || '—' : label;
}

// ── 沙盒 Gate（D4；g5_eval 当前为 skipped 占位记录）─────────────────────────

export function siGateLabel(t: Translate, gate: string): string {
  const key = `selfImprovementPage.gate.${gate || 'unknown'}`;
  const label = t(key);
  return label === key ? gate || '—' : label;
}

// ── 可用操作（状态机终态/中间态决定） ────────────────────────────────────────

export function canApprove(status: string): boolean {
  return status === 'awaiting_governance';
}

export function canReject(status: string): boolean {
  return status === 'awaiting_governance';
}

export function canRollback(status: string): boolean {
  return status === 'applied' || status === 'observing';
}

export function canClose(status: string): boolean {
  return status === 'observing';
}

/** 在途流水线状态（pipeline 在阶段边界消费控制指令）才可下发介入指令（ControlRun）。 */
export function canControl(status: string): boolean {
  return status === 'detected' || status === 'diagnosing' || status === 'patching' || status === 'verifying';
}

/**
 * 解析用户可读错误消息：优先取后端 Kratos envelope（`response.data.message`，
 * 如 409 状态冲突），避免直接展示 axios 英文原始消息。
 */
export function resolveSIErrorMessage(e: unknown): string {
  const data = (e as { response?: { data?: unknown } } | null)?.response?.data;
  const kratosMsg = (data as { message?: unknown } | null)?.message;
  if (typeof kratosMsg === 'string' && kratosMsg) return kratosMsg;
  const msg = (e as { message?: unknown } | null)?.message;
  if (typeof msg === 'string' && msg) return msg;
  return e instanceof Error ? e.message : String(e);
}

/** RFC3339 → 本地短格式（YYYY-MM-DD HH:mm）；空值显示 — */
export function formatSITime(value: string): string {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
