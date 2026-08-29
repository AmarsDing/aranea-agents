<template>
  <q-card flat class="command-center-stat-panel">
    <q-card-section class="q-pb-sm">
      <div class="command-center-stat-panel__header">
        <q-icon
          name="monitor_heart"
          size="16px"
          class="command-center-stat-panel__icon command-center-stat-panel__icon--runner"
        />
        <span class="command-center-stat-panel__title">Runner 运行</span>
      </div>
    </q-card-section>
    <q-card-section class="q-pt-none">
      <div v-if="loading" class="row justify-center q-py-md">
        <q-skeleton type="text" width="60px" />
      </div>
      <template v-else>
        <div class="command-center-stat-panel__gauge">
          <div class="command-center-stat-panel__gauge-item">
            <div class="command-center-stat-panel__gauge-bar">
              <div
                class="command-center-stat-panel__gauge-fill"
                :class="successClass"
                :style="{ width: `${successRate}%` }"
              />
            </div>
            <div class="command-center-stat-panel__gauge-info">
              <span>成功率</span>
              <span class="command-center-stat-panel__gauge-value">{{ successRate.toFixed(1) }}%</span>
            </div>
          </div>
          <div class="command-center-stat-panel__gauge-item">
            <div class="command-center-stat-panel__gauge-bar">
              <div
                class="command-center-stat-panel__gauge-fill"
                :class="errorClass"
                :style="{ width: `${Math.min(errorRate, 100)}%` }"
              />
            </div>
            <div class="command-center-stat-panel__gauge-info">
              <span>错误率</span>
              <span class="command-center-stat-panel__gauge-value">{{ errorRate.toFixed(1) }}%</span>
            </div>
          </div>
        </div>
        <div class="command-center-stat-panel__runner-summary">
          <span>窗口内 {{ totalRuns }} 次运行</span>
          <span v-if="errorRuns > 0" class="command-center-stat-panel__runner-error">{{ errorRuns }} 次错误</span>
        </div>
      </template>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  totalRuns: number;
  errorRuns: number;
  successRate: number;
  errorRate: number;
  loading: boolean;
}>();

function gaugeClass(val: number, inverse = false) {
  if (inverse) {
    if (val >= 10) return 'command-center-stat-panel__gauge-fill--danger';
    if (val >= 5) return 'command-center-stat-panel__gauge-fill--warn';
    return 'command-center-stat-panel__gauge-fill--ok';
  }
  if (val >= 90) return 'command-center-stat-panel__gauge-fill--ok';
  if (val >= 70) return 'command-center-stat-panel__gauge-fill--warn';
  return 'command-center-stat-panel__gauge-fill--danger';
}

const successClass = computed(() => gaugeClass(props.successRate));
const errorClass = computed(() => gaugeClass(props.errorRate, true));
</script>

<style lang="sass">
.command-center-stat-panel__icon--runner
  color: var(--color-accent, #DCA03E)

body.body--dark .command-center-stat-panel__icon--runner
  color: var(--color-accent)
</style>
