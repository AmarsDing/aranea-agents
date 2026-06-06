import type { QTableColumn } from 'quasar';
import type { EvolutionSuggestionView } from '../../features/skills/types';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../../features/ui/registryTableColumns';

// ── Evolution Suggestion Type ──────────────────────────────────────

export const EVO_SUGGESTION_TYPE_FIX_FAILURE = 'fix_failure';
export const EVO_SUGGESTION_TYPE_BOOST_EFFICIENCY = 'boost_efficiency';
export const EVO_SUGGESTION_TYPE_MERGE_DUPLICATE = 'merge_duplicate';

export function evoSuggestionTypeLabel(type?: string): string {
  switch (type) {
    case EVO_SUGGESTION_TYPE_FIX_FAILURE:
      return '修复失败';
    case EVO_SUGGESTION_TYPE_BOOST_EFFICIENCY:
      return '提升效率';
    case EVO_SUGGESTION_TYPE_MERGE_DUPLICATE:
      return '合并重复';
    default:
      return type || '—';
  }
}

export function evoSuggestionTypeColor(type?: string): string {
  switch (type) {
    case EVO_SUGGESTION_TYPE_FIX_FAILURE:
      return 'negative';
    case EVO_SUGGESTION_TYPE_BOOST_EFFICIENCY:
      return 'blue';
    case EVO_SUGGESTION_TYPE_MERGE_DUPLICATE:
      return 'purple';
    default:
      return 'grey';
  }
}

// ── Evolution Suggestion Status ────────────────────────────────────

export function evoSuggestionStatusLabel(status?: string): string {
  switch (status) {
    case 'pending':
      return '待审批';
    case 'approved':
      return '已批准';
    case 'rejected':
      return '已拒绝';
    case 'applied':
      return '已应用';
    default:
      return status || '—';
  }
}

export function evoSuggestionStatusColor(status?: string): string {
  switch (status) {
    case 'pending':
      return 'warning';
    case 'approved':
      return 'positive';
    case 'rejected':
      return 'negative';
    case 'applied':
      return 'info';
    default:
      return 'grey';
  }
}

// ── Lifecycle Status ───────────────────────────────────────────────

export function evoLifecycleStatusLabel(status?: string): string {
  switch (status) {
    case 'draft':
      return '草稿';
    case 'validating':
      return '验证中';
    case 'ready':
      return '就绪';
    case 'applied':
      return '已应用';
    default:
      return status || '—';
  }
}

export function evoLifecycleStatusColor(status?: string): string {
  switch (status) {
    case 'draft':
      return 'grey';
    case 'validating':
      return 'warning';
    case 'ready':
      return 'positive';
    case 'applied':
      return 'info';
    default:
      return 'grey';
  }
}

// ── Table Columns ──────────────────────────────────────────────────

/** EvolutionSuggestionTable 列定义 */
export const EVOLUTION_SUGGESTION_TABLE_COLUMNS: QTableColumn<EvolutionSuggestionView>[] = [
  registryCol<EvolutionSuggestionView>('skillId', 'Skill ID', 'skillId', 'left', REGISTRY_COL_W.name),
  registryCol<EvolutionSuggestionView>('type', '类型', 'type', 'left', REGISTRY_COL_W.category),
  registryCol<EvolutionSuggestionView>('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
  registryCol<EvolutionSuggestionView>('lifecycleStatus', '生命周期', 'lifecycleStatus', 'left', REGISTRY_COL_W.status),
  registryCol<EvolutionSuggestionView>('triggerReason', '触发原因', 'triggerReason', 'left', REGISTRY_COL_W.desc),
  registryCol<EvolutionSuggestionView>('sandboxPassed', '沙箱验证', 'sandboxPassed', 'center', REGISTRY_COL_W.metric),
  registryCol<EvolutionSuggestionView>('createdAt', '创建时间', 'createdAt', 'left', REGISTRY_COL_W.timeWide),
  registryColActions<EvolutionSuggestionView>(),
];
