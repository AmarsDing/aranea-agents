<template>
  <div class="synthesis-result-card">
    <div class="row items-center q-gutter-sm q-mb-sm">
      <div class="synthesis-result-card__icon">
        <q-icon name="auto_awesome" size="18px" />
      </div>
      <div class="col min-width-0">
        <div class="synthesis-result-card__title">{{ t('spirit.synthesisResult') }}</div>
      </div>
      <q-chip dense size="sm" outline :label="strategyLabel" class="synthesis-result-card__strategy" />
      <q-chip
        v-if="successRate !== null"
        dense
        size="sm"
        :outline="successRate < 1"
        :label="successRateLabel"
        :class="successRateClass"
      />
    </div>

    <div class="synthesis-result-card__content">
      <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->
      <div class="synthesis-result-card__text chat-message-prose" v-html="renderedContent" />
    </div>

    <div v-if="result.teamResults.length > 0" class="synthesis-result-card__teams q-mt-sm">
      <div class="synthesis-result-card__teams-title text-caption text-grey q-mb-xs">
        {{ t('spirit.teamResults', { count: result.teamResults.length }) }}
      </div>
      <div v-for="tr in result.teamResults" :key="tr.teamId" class="synthesis-result-card__team-item">
        <div class="synthesis-result-card__team-row row items-center q-gutter-sm">
          <q-icon
            :name="tr.status === 'completed' ? 'check_circle' : 'error'"
            size="14px"
            :color="tr.status === 'completed' ? 'positive' : 'negative'"
          />
          <span class="synthesis-result-card__team-name ellipsis col">{{ tr.teamName }}</span>
          <span class="synthesis-result-card__team-task text-caption text-grey ellipsis">{{ tr.taskName }}</span>
        </div>
        <div v-if="tr.summary" class="synthesis-result-card__team-summary text-caption text-grey q-ml-lg">
          {{ tr.summary }}
        </div>
        <div v-if="tr.keyFindings" class="synthesis-result-card__team-findings text-caption q-ml-lg">
          {{ tr.keyFindings }}
        </div>
      </div>
    </div>

    <div class="synthesis-result-card__meta q-mt-sm">
      <span class="text-caption text-grey">{{ formattedTime }}</span>
    </div>

    <div v-if="evolutionSuggestion" class="synthesis-result-card__evolution q-mt-sm">
      <div class="synthesis-result-card__evolution-title text-caption text-weight-medium q-mb-xs">
        <q-icon name="transform" size="14px" class="q-mr-xs" style="color: var(--color-warning)" />
        {{ t('spirit.evolutionSuggestion') }}
      </div>
      <div class="synthesis-result-card__evolution-body">
        <div class="text-caption text-grey">
          {{ evolutionSuggestion.currentTopology }} → {{ evolutionSuggestion.suggestedTopology }}
        </div>
        <div v-if="evolutionSuggestion.reason" class="text-caption text-grey q-mt-xs">
          {{ evolutionSuggestion.reason }}
        </div>
        <div class="text-caption q-mt-xs" :style="{ color: dqColor }">
          DQ: {{ evolutionSuggestion.dqScore.toFixed(2) }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SynthesisOutput, SynthesisStrategy, EvolutionSuggestion } from '../../features/spirit/types';
import { dqScoreColor } from '../../features/spirit/spiritUi';

const { t } = useI18n();

const props = defineProps<{
  result: SynthesisOutput;
  /** Pre-rendered HTML content from parent (via renderChatMarkdown). */
  renderedContent: string;
  /** Evolution suggestion from DQ analysis. */
  evolutionSuggestion?: EvolutionSuggestion | null;
}>();

const strategyLabel = computed(() => {
  const labels: Record<SynthesisStrategy, string> = {
    template: t('spirit.strategyTemplate'),
    prompt: t('spirit.strategyPrompt'),
    hybrid: t('spirit.strategyHybrid'),
  };
  return labels[props.result.strategy] ?? props.result.strategy;
});

const formattedTime = computed(() => {
  if (!props.result.synthesizedAt) return '';
  try {
    return new Date(props.result.synthesizedAt).toLocaleString();
  } catch {
    return props.result.synthesizedAt;
  }
});

const successRate = computed(() => {
  const teams = props.result.teamResults;
  if (!teams.length) return null;
  const completed = teams.filter((t) => t.status === 'completed').length;
  return completed / teams.length;
});

const successRateLabel = computed(() => {
  if (successRate.value === null) return '';
  return t('spirit.successRate', { rate: Math.round(successRate.value * 100) });
});

const successRateClass = computed(() => {
  if (successRate.value === null) return '';
  if (successRate.value >= 1) return 'synthesis-result-card__rate--full';
  if (successRate.value >= 0.5) return 'synthesis-result-card__rate--partial';
  return 'synthesis-result-card__rate--low';
});

const dqColor = computed(() => dqScoreColor(props.evolutionSuggestion?.dqScore));
</script>

<style scoped lang="sass">
.synthesis-result-card
  padding: var(--space-4)
  border-radius: 16px
  border: 1px solid color-mix(in srgb, var(--color-success) 30%, var(--glass-border))
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

.synthesis-result-card__icon
  display: flex
  align-items: center
  justify-content: center
  width: 28px
  height: 28px
  border-radius: 8px
  background: color-mix(in srgb, var(--color-success) 10%, var(--glass-surface))
  color: var(--color-success)
  flex-shrink: 0

.synthesis-result-card__title
  font-size: var(--text-sm)
  font-weight: 700
  color: var(--color-text-primary)

.synthesis-result-card__strategy
  font-size: 11px

.synthesis-result-card__content
  padding: var(--space-3)
  border-radius: 10px
  background: color-mix(in srgb, var(--glass-surface) 30%, transparent)
  border: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)

.synthesis-result-card__text
  font-size: var(--text-xs)
  line-height: 1.6
  color: var(--color-text-secondary)
  max-height: 300px
  overflow-y: auto

  :deep(img)
    max-width: 100%
    height: auto
    border-radius: 6px

.synthesis-result-card__teams
  padding-top: var(--space-2)
  border-top: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)

.synthesis-result-card__teams-title
  font-weight: 600

.synthesis-result-card__team-row
  padding: 2px 0

.synthesis-result-card__team-item
  padding: 2px 0

.synthesis-result-card__team-name
  font-size: var(--text-xs)
  font-weight: 600
  color: var(--color-text-primary)

.synthesis-result-card__team-task
  max-width: 120px

.synthesis-result-card__team-summary
  font-size: 11px
  white-space: pre-line
  padding: 2px 0
  line-height: 1.5

.synthesis-result-card__team-findings
  font-size: 11px
  color: var(--color-text-tertiary)
  padding: 2px 0
  white-space: pre-line
  line-height: 1.5

.synthesis-result-card__meta
  text-align: right

.synthesis-result-card__rate--full
  color: var(--color-success)
  border-color: color-mix(in srgb, var(--color-success) 40%, var(--glass-border))

.synthesis-result-card__rate--partial
  color: var(--color-warning)
  border-color: color-mix(in srgb, var(--color-warning) 40%, var(--glass-border))

.synthesis-result-card__rate--low
  color: var(--color-danger)
  border-color: color-mix(in srgb, var(--color-danger) 40%, var(--glass-border))

.synthesis-result-card__evolution
  padding-top: var(--space-2)
  border-top: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)

.synthesis-result-card__evolution-title
  display: flex
  align-items: center

.synthesis-result-card__evolution-body
  padding: var(--space-2)
  border-radius: 8px
  background: color-mix(in srgb, var(--color-warning) 5%, var(--glass-surface))
  border: 1px solid color-mix(in srgb, var(--color-warning) 20%, var(--glass-border))
</style>
