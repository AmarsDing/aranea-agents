<template>
  <section class="team-orchestrate-runtime">
    <div class="text-subtitle2 q-mb-sm">运行与容错</div>
    <div class="text-caption app-text-secondary q-mb-md">团队由图编排引擎执行；以下为当前容错配置。</div>

    <q-list dense bordered separator class="rounded-borders">
      <q-item>
        <q-item-section>
          <q-item-label caption>失败策略</q-item-label>
          <q-item-label class="text-body2">{{ failureSummary }}</q-item-label>
        </q-item-section>
      </q-item>
      <q-item v-if="timeoutSec">
        <q-item-section>
          <q-item-label caption>单次运行超时</q-item-label>
          <q-item-label>{{ timeoutText }}</q-item-label>
        </q-item-section>
      </q-item>
    </q-list>

    <div class="text-caption app-text-secondary q-mt-sm">在「编辑 Team」中可调整失败策略与超时。</div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { TeamDefinition } from '../../features/teams/types';
import { failurePolicySummary } from './teamUtils';

const props = defineProps<{
  definition: TeamDefinition | null;
}>();

const failureSummary = computed(() => (props.definition ? failurePolicySummary(props.definition) : '—'));
const timeoutSec = computed(() => props.definition?.timeout_seconds ?? 0);
const timeoutText = computed(() => {
  const sec = timeoutSec.value;
  if (sec >= 60 && sec % 60 === 0) return `${sec / 60} 分钟`;
  return `${sec} 秒`;
});
</script>
