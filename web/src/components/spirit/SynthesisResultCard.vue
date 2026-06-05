<template>
  <div class="synthesis-result-card">
    <div class="row items-center q-gutter-sm q-mb-sm">
      <div class="synthesis-result-card__icon">
        <q-icon name="auto_awesome" size="18px" />
      </div>
      <div class="col min-width-0">
        <div class="synthesis-result-card__title">综合结果</div>
      </div>
      <q-chip dense size="sm" outline :label="strategyLabel" class="synthesis-result-card__strategy" />
    </div>

    <div class="synthesis-result-card__content">
      <div class="synthesis-result-card__text" v-html="renderedContent" />
    </div>

    <div v-if="result.teamResults.length > 0" class="synthesis-result-card__teams q-mt-sm">
      <div class="synthesis-result-card__teams-title text-caption text-grey-6 q-mb-xs">
        团队结果 ({{ result.teamResults.length }})
      </div>
      <div
        v-for="tr in result.teamResults"
        :key="tr.teamId"
        class="synthesis-result-card__team-item"
      >
        <div class="synthesis-result-card__team-row row items-center q-gutter-sm">
          <q-icon
            :name="tr.status === 'completed' ? 'check_circle' : 'error'"
            size="14px"
            :color="tr.status === 'completed' ? 'positive' : 'negative'"
          />
          <span class="synthesis-result-card__team-name ellipsis col">{{ tr.teamName }}</span>
          <span class="synthesis-result-card__team-task text-caption text-grey-6 ellipsis">{{ tr.taskName }}</span>
        </div>
        <div v-if="tr.summary" class="synthesis-result-card__team-summary text-caption text-grey-6 q-ml-lg">
          {{ tr.summary }}
        </div>
        <div v-if="tr.keyFindings" class="synthesis-result-card__team-findings text-caption q-ml-lg" style="white-space: pre-line; max-height: 60px; overflow-y: auto;">
          {{ tr.keyFindings }}
        </div>
      </div>
    </div>

    <div class="synthesis-result-card__meta q-mt-sm">
      <span class="text-caption text-grey-6">{{ formattedTime }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { SynthesisOutput, SynthesisStrategy } from '../../features/spirit/types';
import { renderChatMarkdown } from '../../features/chat/chatMessageMarkdown';

const props = defineProps<{
  result: SynthesisOutput;
}>();

const strategyLabel = computed(() => {
  const labels: Record<SynthesisStrategy, string> = {
    template: '模板合成',
    prompt: 'Prompt 合成',
    hybrid: '混合合成',
  };
  return labels[props.result.strategy] ?? props.result.strategy;
});

const renderedContent = computed(() => {
  return renderChatMarkdown(props.result.content);
});

const formattedTime = computed(() => {
  if (!props.result.synthesizedAt) return '';
  try {
    return new Date(props.result.synthesizedAt).toLocaleString();
  } catch {
    return props.result.synthesizedAt;
  }
});
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
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis
  padding: 1px 0

.synthesis-result-card__team-findings
  font-size: 11px
  color: var(--color-text-tertiary)
  padding: 1px 0

.synthesis-result-card__meta
  text-align: right
</style>
