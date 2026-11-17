/** 批量操作条的交互合同（08-重设计方案 §8.0 业务控件：BulkActionBar）。 */

export type BulkActionTone = 'primary' | 'warn' | 'danger' | 'neutral';

export interface BulkAction {
  /** 上抛给业务层的动作键，父级用它决定调用哪个用例。 */
  key: string;
  label: string;
  icon?: string;
  /** primary=推进类操作，warn=打断类，danger=破坏类，neutral=默认。 */
  tone?: BulkActionTone;
  /** 破坏性动作需二次确认，确认行文案由 confirmLabel 指定。 */
  requiresConfirm?: boolean;
  confirmLabel?: string;
  /** 不可用原因，作为 tooltip，避免「禁用但不说为什么」。 */
  hint?: string;
  disabled?: boolean;
}

/** tone → Quasar 语义色名；neutral 返回 undefined 交给默认样式。 */
export function bulkActionColor(tone: BulkActionTone = 'neutral'): string | undefined {
  if (tone === 'primary') return 'primary';
  if (tone === 'warn') return 'warning';
  if (tone === 'danger') return 'negative';
  return undefined;
}
