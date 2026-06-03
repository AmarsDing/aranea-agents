<template>
  <div class="ctx-breakdown" :class="{ 'ctx-breakdown--dark': isDark }">
    <div class="ctx-breakdown__header row items-center justify-between no-wrap">
      <span class="ctx-breakdown__title text-weight-medium">{{ t('chat.promptBreakdown', 'Prompt 占比分解') }}</span>
      <q-badge v-if="isPrecise" outline color="positive" class="ctx-breakdown__badge">{{
        t('chat.preciseData', '精确')
      }}</q-badge>
      <q-badge v-else outline color="grey" class="ctx-breakdown__badge">{{ t('chat.estimatedData', '估算') }}</q-badge>
    </div>

    <div class="ctx-breakdown__ring-section column items-center q-py-md">
      <q-circular-progress
        :value="breakdown.contextRatio * 100"
        size="80px"
        :thickness="0.22"
        :color="ringColor"
        track-color="grey-3"
        class="ctx-breakdown__ring"
      >
        <div class="ctx-breakdown__ring-inner column items-center justify-center">
          <span class="ctx-breakdown__ring-pct text-weight-bold">{{ pctLabel }}</span>
          <span class="ctx-breakdown__ring-label text-caption">{{ t('chat.contextUsage', '上下文') }}</span>
        </div>
      </q-circular-progress>
    </div>

    <div class="ctx-breakdown__list">
      <div v-for="cat in breakdown.categories" :key="cat.key" class="ctx-breakdown__row row items-center no-wrap">
        <span class="ctx-breakdown__dot" :style="{ backgroundColor: cat.color }" />
        <span class="ctx-breakdown__label col ellipsis">{{ cat.label }}</span>
        <span class="ctx-breakdown__tokens text-caption">{{ formatTokenCount(cat.estTokens) }}</span>
        <span class="ctx-breakdown__pct text-caption text-weight-medium">{{
          breakdownPercent(cat.estTokens, breakdown.totalPromptTokens)
        }}</span>
        <div class="ctx-breakdown__bar-wrap">
          <div
            class="ctx-breakdown__bar-fill"
            :style="{ width: barWidth(cat.estTokens), backgroundColor: cat.color }"
          />
        </div>
      </div>
    </div>

    <q-separator class="q-my-sm" />

    <div class="ctx-breakdown__footer row items-center justify-between text-caption">
      <span
        >{{ t('chat.totalPromptTokens', '总计') }} {{ formatTokenCount(breakdown.totalPromptTokens)
        }}{{ breakdown.contextWindow > 0 ? ` / ${formatTokenCount(breakdown.contextWindow)}` : '' }}</span
      >
      <span v-if="totalCostMicroUsd && totalCostMicroUsd > 0">{{ formatUsdCompact(totalCostMicroUsd) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { PromptBreakdown } from '../../features/chat/contextBreakdown';
import { breakdownPercent } from '../../features/chat/contextBreakdown';
import { formatTokenCount } from '../../features/chat/composerUsageMetrics';
import { formatUsdCompact } from '../../features/usage/moneyFormat';
import { composerContextColor } from '../../features/chat/composerUsageMetrics';

const props = defineProps<{
  breakdown: PromptBreakdown;
  contextStatus?: string;
  totalCostMicroUsd?: number;
  isPrecise?: boolean;
  isDark?: boolean;
}>();

const { t } = useI18n();

const pctLabel = computed(() => `${Math.round(props.breakdown.contextRatio * 100)}%`);

const ringColor = computed(() => {
  const status = props.contextStatus?.trim();
  if (status) return composerContextColor(status);
  if (props.breakdown.contextRatio >= 0.8) return composerContextColor(undefined, props.breakdown.contextRatio);
  return 'accent';
});

function barWidth(tokens: number): string {
  if (props.breakdown.totalPromptTokens <= 0) return '0%';
  return `${Math.min(100, Math.round((tokens / props.breakdown.totalPromptTokens) * 100))}%`;
}
</script>

<style scoped>
.ctx-breakdown {
  min-width: 280px;
  max-width: 340px;
  padding: 12px 16px;
  border-radius: 14px;
  background: var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
  border: 1px solid var(--glass-border);
}

.ctx-breakdown--dark {
  background: var(--glass-surface);
}

.ctx-breakdown__title {
  font-size: 13px;
  color: var(--color-text-primary);
}

.ctx-breakdown__badge {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 4px;
}

.ctx-breakdown__ring {
  position: relative;
}

.ctx-breakdown__ring-inner {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.ctx-breakdown__ring-pct {
  font-size: 16px;
  color: var(--color-text-primary);
}

.ctx-breakdown__ring-label {
  font-size: 10px;
  color: var(--color-text-secondary);
  line-height: 1;
}

.ctx-breakdown__list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.ctx-breakdown__row {
  gap: 8px;
  padding: 2px 0;
}

.ctx-breakdown__dot {
  flex-shrink: 0;
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.ctx-breakdown__label {
  font-size: 12px;
  color: var(--color-text-primary);
  min-width: 0;
}

.ctx-breakdown__tokens {
  flex-shrink: 0;
  color: var(--color-text-secondary);
  font-size: 11px;
}

.ctx-breakdown__pct {
  flex-shrink: 0;
  font-size: 11px;
  color: var(--color-text-primary);
  min-width: 32px;
  text-align: right;
}

.ctx-breakdown__bar-wrap {
  flex-shrink: 0;
  width: 48px;
  height: 4px;
  border-radius: 2px;
  background: color-mix(in srgb, var(--color-text-primary) 10%, transparent);
  overflow: hidden;
}

.ctx-breakdown__bar-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 0.3s ease;
}

.ctx-breakdown__footer {
  color: var(--color-text-secondary);
}
</style>
