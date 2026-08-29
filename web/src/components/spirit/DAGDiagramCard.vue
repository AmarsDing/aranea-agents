<template>
  <div v-if="dagTeams.length > 0 || verificationNodes.length > 0" class="dag-diagram-card">
    <div class="row items-center q-gutter-sm q-mb-sm">
      <div class="dag-diagram-card__icon">
        <q-icon name="account_tree" size="18px" />
      </div>
      <div class="col min-width-0">
        <div class="dag-diagram-card__title">{{ t('spirit.taskDependencyGraph') }}</div>
      </div>
    </div>

    <div class="dag-diagram-card__nodes">
      <div v-for="node in dagNodes" :key="node.teamId" class="dag-diagram-card__node">
        <span class="dag-diagram-card__prefix">{{ node.prefix }}</span>
        <span class="dag-diagram-card__name">{{ node.teamName }}</span>
        <span v-if="node.dependsOn.length > 0" class="dag-diagram-card__deps text-caption text-grey">
          ({{ t('spirit.dependsOn') }}: {{ node.dependsOn.join(', ') }})
        </span>
      </div>
      <div
        v-for="vn in verificationNodes"
        :key="vn.nodeId"
        class="dag-diagram-card__node dag-diagram-card__node--verify"
        :class="verifyNodeClass(vn)"
      >
        <span class="dag-diagram-card__prefix">{{ verifyIcon(vn) }}</span>
        <span class="dag-diagram-card__name">{{ verifyLabel(vn) }}</span>
        <span class="dag-diagram-card__deps text-caption text-grey"> ({{ vn.failureAction }}) </span>
        <span
          v-if="vn.retryCount != null && vn.maxRetries != null"
          class="dag-diagram-card__retry text-caption text-grey"
        >
          {{ t('spirit.retryCount', { current: vn.retryCount, max: vn.maxRetries }) }}
        </span>
        <q-tooltip v-if="vn.status === 'failed' && vn.failureReason" :delay="300">
          {{ vn.failureReason }}
        </q-tooltip>
        <q-tooltip v-else-if="vn.issues && vn.issues.length > 0" :delay="300">
          {{ vn.issues.join('; ') }}
        </q-tooltip>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SpiritTeam, VerificationNode } from '../../features/spirit/types';

const { t } = useI18n();

const props = defineProps<{
  teams: SpiritTeam[];
  /** Verification gate nodes from the orchestration graph. */
  verifications?: VerificationNode[];
}>();

const dagTeams = computed(() => props.teams.filter((t) => t.dagNodeId || (t.dependsOn && t.dependsOn.length > 0)));

const verificationNodes = computed(() => props.verifications ?? []);

const dagNodes = computed(() =>
  dagTeams.value.map((t) => ({
    teamId: t.id,
    teamName: t.teamName || t.taskSummary,
    prefix: t.dependsOn && t.dependsOn.length > 0 ? '⏳' : '▶',
    dependsOn: t.dependsOn ?? [],
  })),
);

const verifyIcon = (vn: VerificationNode) => {
  switch (vn.status) {
    case 'passed':
      return '✓';
    case 'failed':
      return '✗';
    case 'pending':
      return '⏳';
    case 'skipped':
      return '⊘';
    default:
      return '🔍';
  }
};

const verifyLabel = (vn: VerificationNode) => {
  const labels: Record<string, string> = {
    output_format: t('spirit.verifyOutputFormat'),
    task_completion: t('spirit.verifyTaskCompletion'),
    human_approval: t('spirit.verifyHumanApproval'),
  };
  return vn.label ?? labels[vn.type] ?? vn.type;
};

const verifyNodeClass = (vn: VerificationNode) => {
  switch (vn.status) {
    case 'passed':
      return 'dag-diagram-card__node--passed';
    case 'failed':
      return 'dag-diagram-card__node--failed';
    case 'pending':
      return 'dag-diagram-card__node--pending';
    case 'skipped':
      return 'dag-diagram-card__node--skipped';
    default:
      return '';
  }
};
</script>

<style scoped lang="sass">
.dag-diagram-card
  padding: var(--space-4)
  border-radius: 16px
  border: 1px solid var(--glass-border)
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)
  backdrop-filter: blur(var(--glass-blur-default))

.dag-diagram-card__icon
  display: flex
  align-items: center
  justify-content: center
  width: 28px
  height: 28px
  border-radius: 8px
  background: color-mix(in srgb, var(--color-accent) 10%, var(--glass-surface))
  color: var(--color-accent)
  flex-shrink: 0

.dag-diagram-card__title
  font-size: var(--text-sm)
  font-weight: 700
  color: var(--color-text-primary)

.dag-diagram-card__nodes
  display: flex
  flex-direction: column
  gap: var(--space-1)

.dag-diagram-card__node
  display: flex
  align-items: baseline
  gap: 6px
  font-size: var(--text-xs)
  color: var(--color-text-secondary)
  padding: 2px 4px
  border-radius: 4px
  border: 1px solid transparent

.dag-diagram-card__prefix
  flex-shrink: 0

.dag-diagram-card__name
  font-weight: 600
  color: var(--color-text-primary)

.dag-diagram-card__deps
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis

.dag-diagram-card__retry
  white-space: nowrap

.dag-diagram-card__node--verify
  opacity: 0.85
  font-style: italic

.dag-diagram-card__node--passed
  border-color: var(--color-success)
  color: var(--color-success)

  .dag-diagram-card__prefix
    color: var(--color-success)

.dag-diagram-card__node--failed
  border-color: var(--color-danger)
  color: var(--color-danger)

  .dag-diagram-card__prefix
    color: var(--color-danger)

.dag-diagram-card__node--pending
  border-style: dashed
  border-color: var(--color-warning)
  color: var(--color-warning)

  .dag-diagram-card__prefix
    color: var(--color-warning)

.dag-diagram-card__node--skipped
  border-color: var(--color-text-tertiary)
  color: var(--color-text-tertiary)

  .dag-diagram-card__prefix
    color: var(--color-text-tertiary)
</style>
