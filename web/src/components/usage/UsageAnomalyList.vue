<template>
  <q-card flat class="overview-panel overview-anomaly-panel">
    <q-card-section>
      <div class="text-h6 overview-section-title">异常请求</div>
      <div class="text-caption overview-section-caption">失败、超时或取消的模型调用</div>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-card-section class="q-pa-none">
      <q-markup-table flat dense class="overview-anomaly-table">
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
            <td class="overview-anomaly-time">{{ formatTime(row.occurred_at) }}</td>
            <td>{{ row.provider_code }} / {{ row.model_api_id }}</td>
            <td>{{ row.agent_key || "—" }}</td>
            <td>
              <q-badge outline class="overview-status-badge overview-status-badge--error" :label="row.status" />
            </td>
            <td class="overview-anomaly-error ellipsis">{{ row.error_message || "—" }}</td>
          </tr>
          <tr v-if="!rows.length">
            <td colspan="5" class="overview-empty-inline text-center q-pa-lg">暂无异常请求</td>
          </tr>
        </tbody>
      </q-markup-table>
    </q-card-section>
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
