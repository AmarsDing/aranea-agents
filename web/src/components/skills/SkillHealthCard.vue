<template>
  <q-card flat bordered class="skill-health-card">
    <q-card-section>
      <div class="row items-center q-mb-md">
        <q-icon name="monitor_heart" size="sm" class="q-mr-sm" :color="overallColor" />
        <span class="text-subtitle2">Health</span>
        <q-badge rounded :color="overallColor" class="q-ml-sm">{{ overallLabel }}</q-badge>
        <q-space />
        <q-btn flat round dense icon="refresh" size="sm" :loading="loading" @click="emit('refresh')" />
      </div>

      <div v-if="loading" class="column items-center q-pa-md">
        <q-spinner color="primary" size="32px" />
        <div class="text-caption text-grey-7 q-mt-sm">加载中...</div>
      </div>

      <q-banner v-else-if="error" rounded class="bg-negative text-white">
        {{ error }}
        <template #action>
          <q-btn flat color="white" label="重试" @click="emit('refresh')" />
        </template>
      </q-banner>

      <template v-else-if="health">
        <div class="row q-col-gutter-md">
          <div class="col-6">
            <div class="text-caption text-grey-7">7d 成功率</div>
            <div class="text-h6" :class="rateColorClass(health.success_rate_7d)">
              {{ formatPercent(health.success_rate_7d) }}
            </div>
            <div class="text-caption text-grey-7">
              {{ health.success_count_7d }} / {{ health.total_invocations_7d }} 次调用
            </div>
          </div>
          <div class="col-6">
            <div class="text-caption text-grey-7">30d 成功率</div>
            <div class="text-h6" :class="rateColorClass(health.success_rate_30d)">
              {{ formatPercent(health.success_rate_30d) }}
            </div>
            <div class="text-caption text-grey-7">
              {{ health.success_count_30d }} / {{ health.total_invocations_30d }} 次调用
            </div>
          </div>
        </div>

        <q-separator class="q-my-md" />

        <div class="row q-col-gutter-md">
          <div class="col-6">
            <div class="text-caption text-grey-7">7d P95 延迟</div>
            <div class="text-h6" :class="latencyColorClass(health.p95_duration_ms_7d)">
              {{ formatDuration(health.p95_duration_ms_7d) }}
            </div>
          </div>
          <div class="col-6">
            <div class="text-caption text-grey-7">30d P95 延迟</div>
            <div class="text-h6" :class="latencyColorClass(health.p95_duration_ms_30d)">
              {{ formatDuration(health.p95_duration_ms_30d) }}
            </div>
          </div>
        </div>

        <div v-if="health.total_invocations_7d === 0 && health.total_invocations_30d === 0" class="text-caption text-grey-6 q-mt-md">
          暂无调用数据
        </div>
      </template>

      <div v-else class="text-caption text-grey-6 text-center q-pa-md">暂无健康数据</div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { SkillHealthMetric } from '../../features/skills/types';

const props = defineProps<{
  health: SkillHealthMetric | null;
  loading: boolean;
  error: string;
}>();

const emit = defineEmits<{
  refresh: [];
}>();

const overallColor = computed(() => {
  if (!props.health) return 'grey';
  const rate7d = props.health.success_rate_7d;
  if (rate7d >= 0.95) return 'positive';
  if (rate7d >= 0.8) return 'warning';
  return 'negative';
});

const overallLabel = computed(() => {
  if (!props.health) return '无数据';
  const rate7d = props.health.success_rate_7d;
  if (rate7d >= 0.95) return '健康';
  if (rate7d >= 0.8) return '注意';
  return '异常';
});

function rateColorClass(rate: number): string {
  if (rate >= 0.95) return 'text-positive';
  if (rate >= 0.8) return 'text-warning';
  return 'text-negative';
}

function latencyColorClass(ms: number): string {
  if (ms <= 1000) return 'text-positive';
  if (ms <= 5000) return 'text-warning';
  return 'text-negative';
}

function formatPercent(value: number): string {
  if (value === 0) return '0%';
  return `${(value * 100).toFixed(1)}%`;
}

function formatDuration(value: number): string {
  if (value === 0) return '-';
  if (value < 1000) return `${Math.round(value)}ms`;
  return `${(value / 1000).toFixed(1)}s`;
}
</script>
