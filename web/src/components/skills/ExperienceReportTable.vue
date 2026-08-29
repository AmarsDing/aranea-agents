<template>
  <AppRegistryTable
    v-model:expanded="expanded"
    table-class="experience-report-table"
    row-key="id"
    :rows="rows"
    :columns="EXPERIENCE_REPORT_TABLE_COLUMNS"
    :loading="loading"
    hide-pagination
    :pagination="{ rowsPerPage: 0 }"
  >
    <template #body="props">
      <q-tr :props="props">
        <q-td key="_expand" :props="props">
          <q-btn
            v-if="hasDetail(props.row)"
            flat
            dense
            round
            size="sm"
            color="primary"
            :icon="props.expand ? 'expand_less' : 'expand_more'"
            :aria-label="props.expand ? '收起详情' : '展开详情'"
            @click="props.expand = !props.expand"
          />
        </q-td>
        <q-td key="skillName" :props="props">
          <div class="app-registry-cell-primary">{{ props.row.skillName || props.row.skillId || '-' }}</div>
        </q-td>
        <q-td key="result" :props="props">
          <AppStatusChip :status="props.row.isSuccess ? 'success' : 'failed'" />
        </q-td>
        <q-td key="score" :props="props">
          <span>{{ props.row.score != null ? props.row.score.toFixed(2) : '-' }}</span>
        </q-td>
        <q-td key="failureTags" :props="props">
          <template v-if="props.row.failureTags && props.row.failureTags.length > 0">
            <q-chip v-for="tag in props.row.failureTags" :key="tag" dense size="sm" color="negative" text-color="white">
              {{ tag }}
            </q-chip>
          </template>
          <span v-else class="text-grey-7">-</span>
        </q-td>
        <q-td key="flowSummary" :props="props">
          <AppRegistryHoverTip :text="props.row.flowSummary" empty-label="无摘要">
            <div class="min-width-0 ellipsis-2-lines">{{ props.row.flowSummary || '-' }}</div>
          </AppRegistryHoverTip>
        </q-td>
        <q-td key="createdAt" :props="props">
          <div>{{ formatDate(props.row.createdAt) }}</div>
        </q-td>
      </q-tr>
      <q-tr v-if="props.expand" class="experience-report-table__detail-row">
        <q-td colspan="100%">
          <div class="report-detail">
            <div v-if="props.row.rootCauseAnalysis" class="report-detail__block">
              <div class="report-detail__label">根因分析</div>
              <div class="report-detail__content">{{ props.row.rootCauseAnalysis }}</div>
            </div>
            <div v-if="props.row.suggestedFix" class="report-detail__block">
              <div class="report-detail__label">建议修复</div>
              <div class="report-detail__content">{{ props.row.suggestedFix }}</div>
            </div>
            <div v-if="props.row.optimizationAdvice" class="report-detail__block">
              <div class="report-detail__label">优化建议</div>
              <div class="report-detail__content">{{ props.row.optimizationAdvice }}</div>
            </div>
          </div>
        </q-td>
      </q-tr>
    </template>
  </AppRegistryTable>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import AppStatusChip from '../common/AppStatusChip.vue';
import type { ExperienceReportView } from '../../features/skills/types';
import { EXPERIENCE_REPORT_TABLE_COLUMNS } from './experienceReportTableUi';

const props = defineProps<{
  rows: ExperienceReportView[];
  loading: boolean;
}>();

/** 展开行的 row-key 集合（QTable 受控模式） */
const expanded = ref<string[]>([]);

// 筛选/翻页导致行集变化时收起所有展开行，避免详情错位
watch(
  () => props.rows,
  () => {
    expanded.value = [];
  },
);

/** 有根因分析/建议修复/优化建议任一内容的行才提供展开入口 */
function hasDetail(row: ExperienceReportView): boolean {
  return Boolean(row.rootCauseAnalysis || row.suggestedFix || row.optimizationAdvice);
}

function formatDate(value?: string) {
  if (!value) return '-';
  return new Date(value).toLocaleString('zh-CN', { hour12: false });
}
</script>

<style scoped lang="sass">
.experience-report-table__detail-row
  background: var(--overview-neutral-bg)

.report-detail
  display: flex
  flex-direction: column
  gap: 12px
  padding: 14px 18px

.report-detail__label
  font-size: 0.75rem
  font-weight: 600
  letter-spacing: 0.04em
  color: var(--color-text-secondary)
  margin-bottom: 4px

.report-detail__content
  font-size: 0.85rem
  line-height: 1.6
  color: var(--color-text-primary)
  white-space: pre-wrap
  word-break: break-word
</style>
