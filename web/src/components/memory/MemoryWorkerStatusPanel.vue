// Container: approved — worker pipeline counters from GetMemoryWorkerStatus RPC.
<template>
  <q-card flat bordered class="memory-card">
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-h6">Memory Worker 状态</div>
        <div class="text-caption text-grey-7">
          Auto-memory 队列：完成 / dead-letter / LLM fallback / episode backfill / 队列 / 索引健康。
        </div>
      </div>
      <q-btn flat dense icon="refresh" :loading="loading" @click="emit('refresh')" />
    </q-card-section>
    <q-card-section v-if="status" class="row q-col-gutter-md">
      <div v-for="card in cards" :key="card.label" class="col-6 col-md">
        <div class="text-caption text-grey-7">{{ card.label }}</div>
        <div class="text-h5" :class="card.class">{{ card.value }}</div>
      </div>
    </q-card-section>
    <q-card-section v-if="status && queueRows.length" class="q-pt-none">
      <div class="text-caption text-grey-7 q-mb-xs">队列通道</div>
      <q-markup-table flat dense bordered>
        <thead>
          <tr>
            <th>通道</th>
            <th>容量</th>
            <th>在飞</th>
            <th>丢弃</th>
            <th>防抖</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in queueRows" :key="r.lane">
            <td>{{ r.lane }}</td>
            <td>{{ r.capacity }}</td>
            <td>{{ r.inFlight }}</td>
            <td>{{ r.dropped }}</td>
            <td>{{ r.debounced }}</td>
          </tr>
        </tbody>
      </q-markup-table>
    </q-card-section>
    <q-card-section
      v-if="status && (status.fact_index_stale_count || status.fact_index_disabled_count)"
      class="q-pt-none"
    >
      <div class="text-caption text-grey-7 q-mb-xs">索引健康</div>
      <div class="row q-col-gutter-sm">
        <div v-if="status.fact_index_stale_count" class="col-auto">
          <q-badge color="warning">Stale {{ status.fact_index_stale_count }}</q-badge>
        </div>
        <div v-if="status.fact_index_disabled_count" class="col-auto">
          <q-badge color="negative">Disabled {{ status.fact_index_disabled_count }}</q-badge>
        </div>
      </div>
    </q-card-section>
    <q-card-section v-if="status && status.db_available === false" class="q-pt-none">
      <q-badge color="negative">DB 不可用</q-badge>
    </q-card-section>
    <q-card-section v-else-if="!loading" class="text-grey-7 text-caption">Worker 指标暂不可用。</q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { MemoryWorkerStatus } from '../../features/memory/types';

const props = defineProps<{
  status: MemoryWorkerStatus | null;
  loading: boolean;
}>();

const emit = defineEmits<{
  (e: 'refresh'): void;
}>();

const cards = computed(() => {
  if (!props.status) return [];
  const s = props.status;
  return [
    { label: 'Jobs Done', value: s.jobs_done, class: '' },
    {
      label: 'Dead Letter',
      value: s.dead_letter_pending ?? s.jobs_dead,
      class: (s.dead_letter_pending ?? 0) > 0 ? 'text-negative' : '',
    },
    { label: 'LLM Fallback', value: s.llm_fallback_total, class: '' },
    { label: 'Avg Extract (s)', value: s.avg_extraction_seconds.toFixed(2), class: '' },
    { label: 'Episode Backfill', value: s.episode_backfill_total, class: '' },
  ];
});

const queueRows = computed(() => {
  if (!props.status) return [];
  const s = props.status;
  const rows: { lane: string; capacity: number; inFlight: number; dropped: number; debounced: number }[] = [];
  if (s.queue_high)
    rows.push({
      lane: 'High',
      capacity: s.queue_high.capacity,
      inFlight: s.queue_high.in_flight,
      dropped: s.queue_high.dropped_total ?? 0,
      debounced: s.queue_high.debounced_total ?? 0,
    });
  if (s.queue_normal)
    rows.push({
      lane: 'Normal',
      capacity: s.queue_normal.capacity,
      inFlight: s.queue_normal.in_flight,
      dropped: s.queue_normal.dropped_total ?? 0,
      debounced: s.queue_normal.debounced_total ?? 0,
    });
  if (s.queue_low)
    rows.push({
      lane: 'Low',
      capacity: s.queue_low.capacity,
      inFlight: s.queue_low.in_flight,
      dropped: s.queue_low.dropped_total ?? 0,
      debounced: s.queue_low.debounced_total ?? 0,
    });
  return rows;
});
</script>
