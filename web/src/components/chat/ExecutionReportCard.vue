<template>
  <div class="execution-report-card" :class="`execution-report-card--${finalStatus}`">
    <!-- Header: title + status chip + units/duration summary -->
    <div class="execution-report-card__header">
      <div class="execution-report-card__icon">
        <q-icon name="auto_awesome" size="16px" />
      </div>
      <div class="execution-report-card__title">{{ t('chat.executionReport.title') }}</div>
      <q-chip dense size="sm" :label="statusLabel" class="execution-report-card__status" />
      <span v-if="overview" class="execution-report-card__summary">
        {{ unitsSummary }}<template v-if="durationLabel"> · {{ durationLabel }}</template>
      </span>
    </div>

    <!-- Overview row: query (single-line truncated) + tokens -->
    <div v-if="overview" class="execution-report-card__overview">
      <span class="execution-report-card__query" :title="overview.query">{{ overview.query }}</span>
      <span v-if="tokenLabel" class="execution-report-card__tokens">{{ tokenLabel }}</span>
    </div>

    <!-- Intelligent analysis (markdown) / degraded hint -->
    <div v-if="report.degraded" class="execution-report-card__degraded">
      <q-icon name="warning" size="14px" />
      <span>{{ t('chat.executionReport.degradedHint') }}</span>
    </div>
    <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->
    <div v-if="renderedContent" class="execution-report-card__content chat-message-prose" v-html="renderedContent" />

    <!-- Team results (default collapsed) -->
    <q-expansion-item
      v-if="report.teamResults.length > 0"
      dense
      :label="t('chat.executionReport.teamResults', { count: report.teamResults.length })"
      class="execution-report-card__section"
    >
      <div v-for="tr in report.teamResults" :key="tr.teamId || tr.teamName" class="execution-report-card__unit">
        <div class="execution-report-card__unit-row">
          <q-icon
            :name="tr.status === 'completed' ? 'check_circle' : 'error'"
            size="14px"
            :class="tr.status === 'completed' ? 'is-ok' : 'is-err'"
          />
          <span class="execution-report-card__unit-name" :title="tr.teamName">{{ tr.teamName }}</span>
          <span v-if="tr.taskName" class="execution-report-card__unit-task" :title="tr.taskName">{{ tr.taskName }}</span>
          <span class="execution-report-card__unit-duration">{{ formatUnitDuration(tr.durationMs) }}</span>
        </div>
        <div
          v-if="tr.errorMessage || tr.summary"
          class="execution-report-card__unit-summary"
          :title="tr.errorMessage || tr.summary"
        >
          {{ tr.errorMessage || tr.summary }}
        </div>
      </div>
    </q-expansion-item>

    <!-- Deliverables (default collapsed; hidden when empty) -->
    <q-expansion-item
      v-if="report.deliverables.length > 0"
      dense
      :label="t('chat.executionReport.deliverables', { count: report.deliverables.length })"
      class="execution-report-card__section"
    >
      <div
        v-for="d in report.deliverables"
        :key="`${d.nodeId}:${d.unitName}`"
        class="execution-report-card__deliverable"
      >
        <q-icon name="description" size="14px" class="execution-report-card__deliverable-icon" />
        <span class="execution-report-card__deliverable-node">{{ d.nodeId }}</span>
        <span class="execution-report-card__deliverable-unit" :title="d.unitName">{{ d.unitName }}</span>
        <span v-if="d.format" class="execution-report-card__deliverable-format">{{ d.format }}</span>
        <span v-if="d.sizeChars > 0" class="execution-report-card__deliverable-size">
          {{ t('chat.executionReport.chars', { count: d.sizeChars }) }}
        </span>
        <span v-if="d.summary" class="execution-report-card__deliverable-summary" :title="d.summary">
          {{ d.summary }}
        </span>
      </div>
    </q-expansion-item>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ExecutionReportEnvelope } from '../../features/chat/executionReport';
import { renderChatMarkdownForMessage } from '../../features/chat/chatMessageMarkdown';
import { formatDuration } from '../../features/spirit/spiritUi';

const { t } = useI18n();

const props = defineProps<{
  report: ExecutionReportEnvelope;
  /** Step ID — markdown render cache key. */
  stepId: string;
}>();

const overview = computed(() => props.report.overview);

const finalStatus = computed(() => overview.value?.finalStatus ?? 'completed');

const statusLabel = computed(() => {
  switch (finalStatus.value) {
    case 'partial_failure':
      return t('chat.executionReport.statusPartialFailure');
    case 'failed':
      return t('chat.executionReport.statusFailed');
    default:
      return t('chat.executionReport.statusCompleted');
  }
});

const unitsSummary = computed(() => {
  const o = overview.value;
  if (!o || o.totalUnits <= 0) return '';
  return `${o.completedUnits}/${o.totalUnits}`;
});

const durationLabel = computed(() => formatDuration(overview.value?.durationMs));

function formatUnitDuration(ms: number | undefined): string {
  return formatDuration(ms) || '--';
}

function formatTokens(n: number): string {
  return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : `${n}`;
}

const tokenLabel = computed(() => {
  const o = overview.value;
  if (!o || (!o.tokenIn && !o.tokenOut)) return '';
  return t('chat.executionReport.tokenSummary', {
    tokenIn: formatTokens(o.tokenIn),
    tokenOut: formatTokens(o.tokenOut),
  });
});

const renderedContent = computed(() => {
  const content = props.report.content.trim();
  if (!content) return '';
  return renderChatMarkdownForMessage(props.stepId, content, false);
});
</script>

<style lang="sass" scoped>
.execution-report-card
  padding: 12px
  border-radius: 12px
  font-size: 13px
  line-height: 1.5
  margin: 4px 0
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)

  &--completed
    border-color: color-mix(in srgb, var(--color-success) 35%, var(--glass-border))

  &--partial_failure
    border-color: color-mix(in srgb, var(--color-warning) 35%, var(--glass-border))

  &--failed
    border-color: color-mix(in srgb, var(--color-danger) 35%, var(--glass-border))

  &__header
    display: flex
    align-items: center
    gap: 8px

  &__icon
    display: flex
    align-items: center
    justify-content: center
    width: 24px
    height: 24px
    border-radius: 6px
    background: color-mix(in srgb, var(--color-success) 10%, var(--glass-surface))
    color: var(--color-success)
    flex-shrink: 0

  &__title
    flex: 1
    min-width: 0
    font-weight: 700
    color: var(--color-text-primary)

  &__status
    flex-shrink: 0
    font-size: 11px

    .execution-report-card--completed &
      color: var(--color-success)
      border: 1px solid color-mix(in srgb, var(--color-success) 40%, transparent)

    .execution-report-card--partial_failure &
      color: var(--color-warning)
      border: 1px solid color-mix(in srgb, var(--color-warning) 40%, transparent)

    .execution-report-card--failed &
      color: var(--color-danger)
      border: 1px solid color-mix(in srgb, var(--color-danger) 40%, transparent)

  &__summary
    flex-shrink: 0
    font-size: 12px
    color: var(--color-text-secondary)

  &__overview
    display: flex
    align-items: center
    gap: 8px
    margin-top: 6px
    font-size: 12px
    color: var(--color-text-secondary)

  &__query
    flex: 1
    min-width: 0
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__tokens
    flex-shrink: 0
    color: var(--color-text-tertiary)

  &__degraded
    display: flex
    align-items: center
    gap: 6px
    margin-top: 8px
    padding: 6px 10px
    border-radius: 8px
    font-size: 12px
    color: var(--color-warning)
    background: color-mix(in srgb, var(--color-warning) 8%, transparent)
    border: 1px solid color-mix(in srgb, var(--color-warning) 25%, transparent)

  &__content
    margin-top: 8px
    padding: 8px 10px
    border-radius: 8px
    font-size: 13px
    color: var(--color-text-secondary)
    background: color-mix(in srgb, var(--glass-surface) 40%, transparent)
    border: 1px solid color-mix(in srgb, var(--glass-border) 60%, transparent)
    max-height: 320px
    overflow-y: auto

    :deep(p:first-child)
      margin-top: 0

    :deep(p:last-child)
      margin-bottom: 0

  &__section
    margin-top: 8px
    border-radius: 8px
    border: 1px solid color-mix(in srgb, var(--glass-border) 60%, transparent)

    :deep(.q-item)
      min-height: 32px
      padding: 4px 10px
      font-size: 12px
      color: var(--color-text-secondary)

    :deep(.q-expansion-item__content)
      padding: 0 10px 8px

  &__unit
    padding: 4px 0

    & + &
      border-top: 1px solid color-mix(in srgb, var(--glass-border) 40%, transparent)

  &__unit-row
    display: flex
    align-items: center
    gap: 6px

    .is-ok
      color: var(--color-success)

    .is-err
      color: var(--color-danger)

  &__unit-name
    font-weight: 600
    font-size: 12px
    color: var(--color-text-primary)
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap
    max-width: 40%

  &__unit-task
    flex: 1
    min-width: 0
    font-size: 12px
    color: var(--color-text-tertiary)
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__unit-duration
    flex-shrink: 0
    font-size: 11px
    color: var(--color-text-tertiary)

  &__unit-summary
    margin-top: 2px
    padding-left: 20px
    font-size: 12px
    color: var(--color-text-secondary)
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__deliverable
    display: flex
    align-items: center
    gap: 6px
    padding: 4px 0
    font-size: 12px
    color: var(--color-text-secondary)

    & + &
      border-top: 1px solid color-mix(in srgb, var(--glass-border) 40%, transparent)

  &__deliverable-icon
    flex-shrink: 0
    color: var(--color-text-tertiary)

  &__deliverable-node
    flex-shrink: 0
    font-weight: 600
    color: var(--color-text-primary)

  &__deliverable-unit
    flex-shrink: 0
    max-width: 30%
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__deliverable-format,
  &__deliverable-size
    flex-shrink: 0
    color: var(--color-text-tertiary)

  &__deliverable-summary
    flex: 1
    min-width: 0
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap
    color: var(--color-text-tertiary)
</style>
