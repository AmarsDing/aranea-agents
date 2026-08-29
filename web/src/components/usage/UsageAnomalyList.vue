<template>
  <q-card flat class="overview-panel overview-panel--danger">
    <q-card-section>
      <div class="row items-center q-gutter-sm">
        <q-icon name="warning_amber" class="overview-panel__alert-icon overview-panel__alert-icon--danger" />
        <div>
          <div class="text-h6 overview-section-title">异常请求</div>
          <div class="text-caption overview-section-caption">失败、超时或取消的模型调用</div>
        </div>
      </div>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-card-section class="q-pa-none">
      <AppRegistryMarkupTable
        :rows="rows"
        :columns="columns"
        table-class="overview-anomaly-table"
        empty-cell-class="overview-empty-inline text-center q-pa-lg"
      >
        <template #cell-occurred_at="{ value }">
          <span class="overview-anomaly-time">{{ formatTime(String(value)) }}</span>
        </template>
        <template #cell-model_api_id="{ row }"> {{ row.provider_code }} / {{ row.model_api_id }} </template>
        <template #cell-agent_id="{ row }">
          {{ row.agent_key || '—' }}
        </template>
        <template #cell-status="{ row }">
          <AppRegistryHoverTip :text="String(row.error_message ?? '')" empty-label="暂无错误信息">
            <AppStatusChip :status="String(row.status)" />
          </AppRegistryHoverTip>
        </template>
        <template #empty>暂无异常请求</template>
      </AppRegistryMarkupTable>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import AppRegistryMarkupTable from '../layout/AppRegistryMarkupTable.vue';
import AppStatusChip from '../common/AppStatusChip.vue';
import { usageAnomalyColumns } from './usageAnomalyTableUi';
import type { ModelTokenUsageEvent } from '../../features/usage/types';

defineProps<{
  rows: ModelTokenUsageEvent[];
}>();

const columns = usageAnomalyColumns;

function formatTime(value: string) {
  if (!value) return '—';
  try {
    return new Date(value).toLocaleString();
  } catch {
    return value;
  }
}
</script>
