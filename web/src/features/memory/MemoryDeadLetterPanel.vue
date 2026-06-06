// Container: approved — dead-letter queue management with replay/abandon actions.
<template>
  <q-card flat bordered class="memory-card">
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-h6">Dead-Letter 队列</div>
        <div class="text-caption text-grey-7">失败的记忆提取任务，可重试或放弃。</div>
      </div>
      <q-btn flat dense icon="refresh" :loading="loading" @click="load" />
    </q-card-section>
    <q-card-section v-if="rows.length">
      <q-markup-table flat dense bordered>
        <thead>
          <tr>
            <th>ID</th>
            <th>Session</th>
            <th>App</th>
            <th>优先级</th>
            <th>原因</th>
            <th>尝试</th>
            <th>状态</th>
            <th>失败时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in rows" :key="r.id">
            <td>{{ r.id }}</td>
            <td class="ellipsis-cell" style="max-width: 120px">{{ r.session_id }}</td>
            <td>{{ r.app_name }}</td>
            <td>
              <q-badge :color="priorityColor(r.priority)">{{ priorityLabel(r.priority) }}</q-badge>
            </td>
            <td class="ellipsis-cell" style="max-width: 200px" :title="r.drop_reason">{{ r.drop_reason }}</td>
            <td>{{ r.attempts }}</td>
            <td>
              <q-badge :color="stateColor(r.state)">{{ r.state }}</q-badge>
            </td>
            <td>{{ formatTime(r.failed_at) }}</td>
            <td class="q-gutter-xs">
              <q-btn
                v-if="r.state === 'pending'"
                flat
                dense
                color="primary"
                icon="replay"
                size="sm"
                @click="replay(r.id)"
              >
                <q-tooltip>重试</q-tooltip>
              </q-btn>
              <q-btn
                v-if="r.state === 'pending'"
                flat
                dense
                color="negative"
                icon="delete_outline"
                size="sm"
                @click="abandon(r.id)"
              >
                <q-tooltip>放弃</q-tooltip>
              </q-btn>
            </td>
          </tr>
        </tbody>
      </q-markup-table>
    </q-card-section>
    <q-card-section v-else-if="!loading" class="text-grey-7 text-caption">暂无 Dead-Letter 条目。</q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import type { MemoryDeadLetterEntry } from './types';
import { useMemoryApi } from './composables/useMemoryApi';
const { listMemoryDeadLetters } = useMemoryApi();

const emit = defineEmits<{
  (e: 'replay', id: number): void;
  (e: 'abandon', id: number): void;
}>();

const rows = ref<MemoryDeadLetterEntry[]>([]);
const loading = ref(false);

function priorityLabel(p: number) {
  if (p >= 2) return 'High';
  if (p === 1) return 'Normal';
  return 'Low';
}

function priorityColor(p: number) {
  if (p >= 2) return 'negative';
  if (p === 1) return 'warning';
  return 'grey';
}

function stateColor(s: string) {
  if (s === 'pending') return 'warning';
  if (s === 'replayed') return 'positive';
  if (s === 'abandoned') return 'grey';
  return 'dark';
}

function formatTime(t: string) {
  if (!t) return '-';
  try {
    return new Date(t).toLocaleString();
  } catch {
    return t;
  }
}

async function load() {
  loading.value = true;
  try {
    rows.value = await listMemoryDeadLetters('pending', 50);
  } catch {
    rows.value = [];
  } finally {
    loading.value = false;
  }
}

function replay(id: number) {
  emit('replay', id);
}

function abandon(id: number) {
  emit('abandon', id);
}

onMounted(load);

defineExpose({ load });
</script>
