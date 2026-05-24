// Container: approved — worker pipeline counters from GetMemoryWorkerStatus RPC.
<template>
  <q-card flat bordered class="memory-card">
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-h6">Memory Worker 状态</div>
        <div class="text-caption text-grey-7">Auto-memory 队列：完成 / dead-letter / LLM fallback / episode backfill。</div>
      </div>
      <q-btn flat dense icon="refresh" :loading="loading" @click="load" />
    </q-card-section>
    <q-card-section v-if="status" class="row q-col-gutter-md">
      <div v-for="card in cards" :key="card.label" class="col-6 col-md">
        <div class="text-caption text-grey-7">{{ card.label }}</div>
        <div class="text-h5">{{ card.value }}</div>
      </div>
    </q-card-section>
    <q-card-section v-else-if="!loading" class="text-grey-7 text-caption">Worker 指标暂不可用。</q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import type { MemoryWorkerStatus } from "./types";
import { getMemoryWorkerStatus } from "./api";

const status = ref<MemoryWorkerStatus | null>(null);
const loading = ref(false);

const cards = computed(() => {
  if (!status.value) return [];
  const s = status.value;
  return [
    { label: "Jobs Done", value: s.jobs_done },
    { label: "Dead Letter", value: s.jobs_dead },
    { label: "LLM Fallback", value: s.llm_fallback_total },
    { label: "Avg Extract (s)", value: s.avg_extraction_seconds.toFixed(2) },
    { label: "Episode Backfill", value: s.episode_backfill_total }
  ];
});

async function load() {
  loading.value = true;
  try {
    status.value = await getMemoryWorkerStatus();
  } catch {
    status.value = null;
  } finally {
    loading.value = false;
  }
}

onMounted(load);

defineExpose({ load });
</script>
