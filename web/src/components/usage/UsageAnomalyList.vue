<template>
  <q-card flat bordered class="usage-panel">
    <q-card-section>
      <div class="text-h6">异常请求</div>
      <div class="text-caption text-grey-7">失败、超时或取消的模型调用</div>
    </q-card-section>
    <q-markup-table flat dense>
      <thead>
        <tr>
          <th class="text-left">时间</th>
          <th class="text-left">模型</th>
          <th class="text-left">Agent</th>
          <th class="text-left">状态</th>
          <th class="text-left">错误</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in rows" :key="row.id">
          <td>{{ formatTime(row.occurred_at) }}</td>
          <td>{{ row.provider_code }} / {{ row.model_api_id }}</td>
          <td>{{ row.agent_key || "—" }}</td>
          <td><q-badge color="negative" :label="row.status" /></td>
          <td class="ellipsis" style="max-width: 220px">{{ row.error_message || "—" }}</td>
        </tr>
        <tr v-if="!rows.length">
          <td colspan="5" class="text-grey-6 text-center q-pa-md">暂无异常请求</td>
        </tr>
      </tbody>
    </q-markup-table>
  </q-card>
</template>

<script setup lang="ts">
import type { ModelTokenUsageEvent } from "../../features/usage/types";

defineProps<{
  rows: ModelTokenUsageEvent[];
}>();

function formatTime(value: string) {
  if (!value) return "—";
  try {
    return new Date(value).toLocaleString();
  } catch {
    return value;
  }
}
</script>

<style scoped>
.usage-panel {
  border-radius: 18px;
}
</style>
