<template>
  <q-card v-if="!cacheHitDenied" flat class="overview-panel">
    <q-inner-loading :showing="cacheHitLoading" color="primary">
      <q-spinner size="32px" />
    </q-inner-loading>
    <q-card-section>
      <div class="row items-center q-gutter-sm">
        <q-icon name="cached" class="overview-panel__alert-icon" />
        <div class="col">
          <div class="text-h6 overview-section-title">{{ t('overviewPage.cacheHitTitle') }}</div>
          <div class="text-caption overview-section-caption">{{ t('overviewPage.cacheHitCaption') }}</div>
        </div>
        <q-btn-toggle
          v-model="windowHours"
          dense
          outline
          no-caps
          toggle-color="primary"
          :options="windowOptions"
          @update:model-value="reload"
        />
      </div>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-card-section>
      <div v-if="!cacheHitStats.length" class="overview-empty overview-empty--compact">
        {{ t('overviewPage.cacheHitEmpty') }}
      </div>
      <q-markup-table v-else flat dense class="cache-hit-table">
        <thead>
          <tr>
            <th class="text-left">{{ t('overviewPage.cacheHitColProvider') }}</th>
            <th class="text-left">{{ t('overviewPage.cacheHitColModel') }}</th>
            <th class="text-right">{{ t('overviewPage.cacheHitColSamples') }}</th>
            <th class="text-right">{{ t('overviewPage.cacheHitColP50') }}</th>
            <th class="text-right">
              {{ t('overviewPage.cacheHitColWeighted') }}
              <q-icon name="help_outline" size="14px" class="q-ml-xs text-grey">
                <q-tooltip>{{ t('overviewPage.cacheHitWeightedHint') }}</q-tooltip>
              </q-icon>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in cacheHitStats" :key="`${row.provider}:${row.model}`">
            <td>{{ row.provider }}</td>
            <td>{{ row.model }}</td>
            <td class="text-right">{{ row.samples }}</td>
            <td
              class="text-right"
              :class="row.p50_ratio < LOW_WATER ? 'text-negative text-weight-bold' : 'text-positive'"
            >
              {{ formatRatio(row.p50_ratio) }}
            </td>
            <td class="text-right text-grey-7">{{ formatRatio(row.weighted_ratio) }}</td>
          </tr>
        </tbody>
      </q-markup-table>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { storeToRefs } from 'pinia';
import { useUsageStore } from '../../stores/usage';

// 与 llm.cache_hit_ratio_low 告警阈值一致（29-token §9.4，默认 0.5）。
const LOW_WATER = 0.5;

const { t } = useI18n();
const usageStore = useUsageStore();
const { cacheHitStats, cacheHitLoading, cacheHitDenied } = storeToRefs(usageStore);
const windowHours = ref(24);

const windowOptions = computed(() => [
  { label: t('overviewPage.cacheHitWindow1h'), value: 1 },
  { label: t('overviewPage.cacheHitWindow24h'), value: 24 },
  { label: t('overviewPage.cacheHitWindow7d'), value: 168 },
]);

function formatRatio(v: number): string {
  return `${(v * 100).toFixed(1)}%`;
}

function reload() {
  void usageStore.loadCacheHitStats(windowHours.value);
}

onMounted(reload);
</script>

<style scoped>
.cache-hit-table {
  background: transparent;
}
</style>
