<template>
  <div class="ov-verdict" :class="`ov-verdict--${verdict}`">
    <div class="ov-verdict__main">
      <span class="ov-verdict__beacon" aria-hidden="true" />
      <div class="ov-verdict__text">
        <div class="ov-verdict__headline">{{ headline }}</div>
        <div class="ov-verdict__sub">{{ subline }}</div>
      </div>
      <div class="ov-verdict__chips">
        <span
          v-for="chip in chips"
          :key="chip.key"
          class="ov-verdict__chip"
          :class="`ov-verdict__chip--${chip.severity}`"
        >
          <q-icon :name="chip.icon" size="13px" />
          {{ chip.label }}
        </span>
      </div>
    </div>
    <div class="ov-verdict__side">
      <span class="ov-verdict__live" :class="{ 'ov-verdict__live--stale': stale }">
        <span class="ov-verdict__live-dot" aria-hidden="true" />
        {{ t('overviewPage.v2.live') }}
      </span>
      <span v-if="updatedAt" class="ov-verdict__updated">{{
        t('overviewPage.v2.updatedAt', { time: updatedAt })
      }}</span>
      <slot name="filters" />
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :loading="loading"
        :title="t('overviewPage.v2.refresh')"
        @click="$emit('refresh')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { OvVerdict, OvVerdictChip } from './ovTypes';

withDefaults(
  defineProps<{
    verdict: OvVerdict;
    headline: string;
    subline: string;
    chips: OvVerdictChip[];
    /** 预格式化的时间文本（'—' 或非空）。 */
    updatedAt?: string;
    stale?: boolean;
    loading?: boolean;
  }>(),
  { updatedAt: '', stale: false, loading: false },
);

defineEmits<{ refresh: [] }>();

const { t } = useI18n();
</script>
