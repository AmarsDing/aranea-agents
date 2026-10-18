import type { QTableColumn } from 'quasar';
import { i18n } from '../../i18n';
import type { SkillEvolutionView } from '../../features/skills/types';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../../features/ui/registryTableColumns';

// ── Evolution Suggestion Type ──────────────────────────────────────

export const EVO_SUGGESTION_TYPE_FIX_FAILURE = 'fix_failure';
export const EVO_SUGGESTION_TYPE_BOOST_EFFICIENCY = 'boost_efficiency';
export const EVO_SUGGESTION_TYPE_MERGE_DUPLICATE = 'merge_duplicate';

export function evoSuggestionTypeLabel(type?: string): string {
  switch (type) {
    case EVO_SUGGESTION_TYPE_FIX_FAILURE:
      return i18n.global.t('evolutionSuggestionsPage.typeFixFailure');
    case EVO_SUGGESTION_TYPE_BOOST_EFFICIENCY:
      return i18n.global.t('evolutionSuggestionsPage.typeBoostEfficiency');
    case EVO_SUGGESTION_TYPE_MERGE_DUPLICATE:
      return i18n.global.t('evolutionSuggestionsPage.typeMergeDuplicate');
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

// ── Evolution Target Type ──────────────────────────────────────────

export function evoTargetTypeLabel(targetType?: string): string {
  switch (targetType) {
    case 'skill':
      return 'Skill';
    case 'agent':
      return 'Agent';
    default:
      return targetType || '—';
  }
}

export function evoTargetTypeColor(targetType?: string): string {
  switch (targetType) {
    case 'skill':
      return 'teal';
    case 'agent':
      return 'deep-purple';
    default:
      return 'grey';
  }
}

// ── Evolution Action Type ──────────────────────────────────────────

export function evoActionTypeLabel(actionType?: string): string {
  switch (actionType) {
    case 'create_skill':
      return i18n.global.t('evolutionSuggestionsPage.actionCreateSkill');
    case 'improve_skill':
      return i18n.global.t('evolutionSuggestionsPage.actionImproveSkill');
    case 'merge_skill':
      return i18n.global.t('evolutionSuggestionsPage.actionMergeSkill');
    case 'evolve_agent':
      return i18n.global.t('evolutionSuggestionsPage.actionEvolveAgent');
    default:
      return actionType || '—';
  }
}

export function evoActionTypeColor(actionType?: string): string {
  switch (actionType) {
    case 'create_skill':
      return 'positive';
    case 'improve_skill':
      return 'blue';
    case 'merge_skill':
      return 'purple';
    case 'evolve_agent':
      return 'deep-purple';
    default:
      return 'grey';
  }
}

// ── Evolution Suggestion Status ────────────────────────────────────

export function evoSuggestionStatusLabel(status?: string): string {
  switch (status) {
    case 'pending':
      return i18n.global.t('evolutionSuggestionsPage.statusPending');
    case 'approved':
      return i18n.global.t('evolutionSuggestionsPage.statusApproved');
    case 'rejected':
      return i18n.global.t('evolutionSuggestionsPage.statusRejected');
    case 'applied':
      return i18n.global.t('evolutionSuggestionsPage.statusApplied');
    case 'registered':
      return i18n.global.t('evolutionSuggestionsPage.statusRegistered');
    case 'expired':
      return i18n.global.t('evolutionSuggestionsPage.statusExpired');
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
    case 'registered':
      return 'teal';
    case 'expired':
      return 'grey';
    default:
      return 'grey';
  }
}

// ── Lifecycle Status ───────────────────────────────────────────────

export function evoLifecycleStatusLabel(status?: string): string {
  switch (status) {
    case 'draft':
      return i18n.global.t('evolutionSuggestionsPage.lifecycleDraft');
    case 'validating':
      return i18n.global.t('evolutionSuggestionsPage.lifecycleValidating');
    case 'ready':
      return i18n.global.t('evolutionSuggestionsPage.lifecycleReady');
    case 'applied':
      return i18n.global.t('evolutionSuggestionsPage.lifecycleApplied');
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

/** Unified Evolution Table 列定义 */
export const EVOLUTION_SUGGESTION_TABLE_COLUMNS: QTableColumn<SkillEvolutionView>[] = [
  registryCol<SkillEvolutionView>(
    'targetType',
    i18n.global.t('evolutionSuggestionsPage.colTargetType'),
    'targetType',
    'left',
    REGISTRY_COL_W.category,
  ),
  registryCol<SkillEvolutionView>(
    'targetId',
    i18n.global.t('evolutionSuggestionsPage.colTargetId'),
    'targetId',
    'left',
    REGISTRY_COL_W.name,
  ),
  registryCol<SkillEvolutionView>(
    'actionType',
    i18n.global.t('evolutionSuggestionsPage.colActionType'),
    'actionType',
    'left',
    REGISTRY_COL_W.category,
  ),
  registryCol<SkillEvolutionView>(
    'status',
    i18n.global.t('evolutionSuggestionsPage.colStatus'),
    'status',
    'left',
    REGISTRY_COL_W.status,
  ),
  registryCol<SkillEvolutionView>(
    'lifecycleStatus',
    i18n.global.t('evolutionSuggestionsPage.colLifecycle'),
    'lifecycleStatus',
    'left',
    REGISTRY_COL_W.status,
  ),
  registryCol<SkillEvolutionView>(
    'triggerReason',
    i18n.global.t('evolutionSuggestionsPage.colTriggerReason'),
    'triggerReason',
    'left',
    REGISTRY_COL_W.desc,
  ),
  registryCol<SkillEvolutionView>(
    'sandboxPassed',
    i18n.global.t('evolutionSuggestionsPage.colSandboxPassed'),
    'sandboxPassed',
    'center',
    REGISTRY_COL_W.metric,
  ),
  registryCol<SkillEvolutionView>(
    'createdAt',
    i18n.global.t('evolutionSuggestionsPage.colCreatedAt'),
    'createdAt',
    'left',
    REGISTRY_COL_W.timeWide,
  ),
  registryColActions<SkillEvolutionView>(),
];
