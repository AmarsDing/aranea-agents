<template>
  <q-card flat bordered class="monitor-card q-mb-md">
    <q-card-section class="row items-center">
      <div class="text-h6 text-weight-bold">Runner 指标</div>
      <q-space />
      <q-select
        v-model="windowMinutes"
        dense
        outlined
        emit-value
        map-options
        :options="windowOptions"
        label="窗口"
        style="min-width: 120px"
        class="q-mr-sm"
      />
      <q-btn flat rounded icon="refresh" label="刷新" :loading="loading" @click="load" />
    </q-card-section>
    <q-separator />
    <q-card-section v-if="metrics">
      <div class="row q-col-gutter-md">
        <div class="col-6 col-md-3">
          <div class="text-caption text-grey">总运行次数</div>
          <div class="text-h5 text-weight-bold">{{ metrics.total_runs }}</div>
        </div>
        <div class="col-6 col-md-3">
          <div class="text-caption text-grey">错误次数</div>
          <div class="text-h5 text-weight-bold text-negative">{{ metrics.error_runs }}</div>
        </div>
        <div class="col-6 col-md-3">
          <div class="text-caption text-grey">错误率</div>
          <div class="text-h5 text-weight-bold">{{ formatPercent(metrics.error_rate) }}</div>
        </div>
        <div class="col-6 col-md-3">
          <div class="text-caption text-grey">成功率</div>
          <div class="text-h5 text-weight-bold">{{ formatPercent(metrics.success_rate) }}</div>
        </div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { getRunnerMetrics, type RunnerMetricsSummary } from "../../features/monitor/api";

const windowMinutes = ref(60);
const metrics = ref<RunnerMetricsSummary | null>(null);
const loading = ref(false);

const windowOptions = [
  { label: "15 分钟", value: 15 },
  { label: "1 小时", value: 60 },
  { label: "6 小时", value: 360 },
  { label: "24 小时", value: 1440 }
];

function formatPercent(v: number): string {
  if (!Number.isFinite(v)) return "-";
  return `${(v * 100).toFixed(1)}%`;
}

async function load() {
  loading.value = true;
  try {
    metrics.value = await getRunnerMetrics(windowMinutes.value);
  } finally {
    loading.value = false;
  }
}

watch(windowMinutes, () => void load());
onMounted(() => void load());
</script>
