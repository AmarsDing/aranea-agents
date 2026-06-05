<template>
  <AppRegistryTable
    table-class="experience-report-table"
    row-key="id"
    :rows="rows"
    :columns="EXPERIENCE_REPORT_TABLE_COLUMNS"
    :loading="loading"
    hide-pagination
  >
    <template #body-cell-skillId="props">
      <q-td :props="props">
        <div class="app-registry-cell-primary">{{ props.row.skillId || '-' }}</div>
      </q-td>
    </template>
    <template #body-cell-result="props">
      <q-td :props="props">
        <q-badge rounded :color="props.row.isSuccess ? 'positive' : 'negative'">
          {{ props.row.isSuccess ? '成功' : '失败' }}
        </q-badge>
      </q-td>
    </template>
    <template #body-cell-score="props">
      <q-td :props="props">
        <span>{{ props.row.score != null ? props.row.score.toFixed(2) : '-' }}</span>
      </q-td>
    </template>
    <template #body-cell-failureTags="props">
      <q-td :props="props">
        <template v-if="props.row.failureTags && props.row.failureTags.length > 0">
          <q-chip
            v-for="tag in props.row.failureTags"
            :key="tag"
            dense
            size="sm"
            color="negative"
            text-color="white"
          >
            {{ tag }}
          </q-chip>
        </template>
        <span v-else class="text-grey-7">-</span>
      </q-td>
    </template>
    <template #body-cell-flowSummary="props">
      <q-td :props="props">
        <AppRegistryHoverTip :text="props.row.flowSummary" empty-label="无摘要">
          <div class="min-width-0 ellipsis-2-lines">{{ props.row.flowSummary || '-' }}</div>
        </AppRegistryHoverTip>
      </q-td>
    </template>
    <template #body-cell-createdAt="props">
      <q-td :props="props">
        <div>{{ formatDate(props.row.createdAt) }}</div>
      </q-td>
    </template>
  </AppRegistryTable>
</template>

<script setup lang="ts">
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import type { ExperienceReport } from '../../services/kratos/skill_intelligence/v1/index';
import { EXPERIENCE_REPORT_TABLE_COLUMNS } from './experienceReportTableUi';

defineProps<{
  rows: ExperienceReport[];
  loading: boolean;
}>();

function formatDate(value?: string) {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}
</script>
