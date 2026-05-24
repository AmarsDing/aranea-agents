<template>
  <div>
    <div v-if="loading" class="column items-center q-py-lg">
      <q-spinner color="primary" size="32px" />
    </div>
    <div v-else-if="error" class="text-negative q-pa-md">{{ error }}</div>
    <div v-else-if="!runs.length" class="text-grey-7 q-pa-md">暂无 Run 记录</div>
    <q-list v-else separator>
      <q-item v-for="run in runs" :key="run.id" class="run-item">
        <q-item-section>
          <q-item-label overline class="text-grey-7">{{ run.phase }} · {{ run.source || "native" }}</q-item-label>
          <q-item-label>{{ run.id }}</q-item-label>
          <q-item-label caption class="text-grey-6">
            Turn {{ run.turn_id || "—" }}
            <span v-if="run.started_at"> · {{ formatDate(run.started_at) }}</span>
            <span v-if="run.error_message" class="text-negative"> · {{ run.error_message }}</span>
          </q-item-label>
        </q-item-section>
        <q-item-section side class="text-right">
          <q-badge :color="phaseColor(run.phase)" outline>{{ run.phase }}</q-badge>
        </q-item-section>
      </q-item>
    </q-list>

    <div v-if="total > pageSize" class="row justify-center q-mt-md q-gutter-sm">
      <q-btn flat dense :disable="offset <= 0" icon="chevron_left" @click="prevPage" />
      <span class="self-center text-caption text-grey-7">{{ pageLabel }}</span>
      <q-btn flat dense :disable="offset + pageSize >= total" icon="chevron_right" @click="nextPage" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { toRef } from "vue";
import { useSessionRunsPanel } from "../../features/session/useSessionRunsPanel";

const props = defineProps<{ sessionId: string }>();

const { runs, total, loading, error, offset, pageSize, pageLabel, prevPage, nextPage } = useSessionRunsPanel(
  toRef(() => props.sessionId)
);

function formatDate(value: string) {
  if (!value) return "—";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString();
}

function phaseColor(phase: string) {
  if (phase === "completed") return "positive";
  if (phase === "failed") return "negative";
  if (phase === "durable" || phase === "escalating") return "warning";
  return "primary";
}
</script>

<style scoped>
.run-item {
  border-radius: 12px;
  margin-bottom: 4px;
}
</style>
