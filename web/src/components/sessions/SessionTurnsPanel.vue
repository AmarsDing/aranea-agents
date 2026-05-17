<template>
  <div>
    <div v-if="loading" class="column items-center q-py-lg">
      <q-spinner color="primary" size="32px" />
    </div>
    <div v-else-if="error" class="text-negative q-pa-md">{{ error }}</div>
    <div v-else-if="!turns.length" class="text-grey-7 q-pa-md">暂无 Turn 记录</div>
    <q-list v-else separator>
      <q-item v-for="turn in turns" :key="turn.id" class="turn-item">
        <q-item-section side>
          <q-badge :color="turnStatusColor(turn.status)" outline>{{ turn.turn_index }}</q-badge>
        </q-item-section>
        <q-item-section>
          <q-item-label overline class="text-grey-7">
            Turn #{{ turn.turn_index }}
            <span v-if="turn.final_provider || turn.final_model" class="q-ml-sm">
              {{ turn.final_provider }}/{{ turn.final_model }}
            </span>
          </q-item-label>
          <q-item-label v-if="turn.final_content_preview" class="turn-preview">
            {{ turn.final_content_preview }}
          </q-item-label>
          <q-item-label caption class="text-grey-6">
            {{ formatDate(turn.started_at) }}
            <span v-if="turn.duration_ms"> · 耗时 {{ formatDuration(turn.duration_ms) }}</span>
            <span v-if="turn.error_message" class="text-negative"> · {{ turn.error_message }}</span>
          </q-item-label>
        </q-item-section>
        <q-item-section side class="text-right">
          <div class="text-caption text-grey-7">
            <div>IN {{ turn.input_tokens }} · OUT {{ turn.output_tokens }}</div>
            <div v-if="turn.total_tokens">Total {{ turn.total_tokens }}</div>
            <div class="q-mt-xs">
              <q-badge v-if="turn.model_call_count" color="blue-grey" outline class="q-mr-xs">模型 {{ turn.model_call_count }}</q-badge>
              <q-badge v-if="turn.tool_call_count" color="info" outline class="q-mr-xs">工具 {{ turn.tool_call_count }}</q-badge>
              <q-badge v-if="turn.skill_call_count" color="deep-purple" outline>技能 {{ turn.skill_call_count }}</q-badge>
            </div>
          </div>
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
import { onMounted, ref, computed } from "vue";
import type { SessionTurn } from "../../features/session/api";
import { useSessionStore } from "../../stores/session/index";

const props = defineProps<{ sessionId: string }>();

const sessionStore = useSessionStore();

const turns = ref<SessionTurn[]>([]);
const total = ref(0);
const loading = ref(false);
const error = ref("");
const offset = ref(0);
const pageSize = 20;

const pageLabel = computed(() => `${offset.value + 1}–${Math.min(offset.value + pageSize, total.value)} / ${total.value}`);

function formatDate(value: string) {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function formatDuration(ms: number) {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

function turnStatusColor(status: string) {
  if (status === "completed") return "positive";
  if (status === "running") return "warning";
  if (status === "failed") return "negative";
  return "grey";
}

async function loadTurns() {
  loading.value = true;
  error.value = "";
  try {
    const result = await sessionStore.fetchTurns(props.sessionId, pageSize, offset.value);
    turns.value = result.items;
    total.value = result.total;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 Turn 失败";
  } finally {
    loading.value = false;
  }
}

function prevPage() {
  offset.value = Math.max(0, offset.value - pageSize);
  loadTurns();
}

function nextPage() {
  offset.value += pageSize;
  loadTurns();
}

onMounted(loadTurns);
</script>

<style scoped>
.turn-item {
  border-radius: 12px;
  margin-bottom: 4px;
}

.turn-preview {
  max-width: 480px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-primary);
}
</style>
