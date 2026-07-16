<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
      <q-card-section class="text-h6">用例结果 · {{ runId }}</q-card-section>
      <q-card-section class="app-dialog-body q-pt-none">
        <AppRegistryTable
          :shell="false"
          :data-shell="true"
          :rows="rows"
          :columns="columns"
          row-key="id"
          :loading="loading"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
        >
          <template #body-cell-case_id="slotProps">
            <q-td :props="slotProps">
              <AppRegistryHoverTip :text="slotProps.row.error_message">
                <span class="app-registry-cell-primary ellipsis">{{ slotProps.row.case_id }}</span>
              </AppRegistryHoverTip>
            </q-td>
          </template>
          <template #body-cell-human_pass="slotProps">
            <q-td :props="slotProps">
              <q-btn-toggle
                :model-value="passValue(slotProps.row)"
                toggle-color="primary"
                dense
                unelevated
                :options="[
                  { label: '-', value: 'unset' },
                  { label: 'Pass', value: 'pass' },
                  { label: 'Fail', value: 'fail' },
                ]"
                @update:model-value="(v: string) => onPassChange(slotProps.row, v)"
              />
            </q-td>
          </template>
          <template #body-cell-human_score="slotProps">
            <q-td :props="slotProps">
              <q-input
                dense
                outlined
                type="number"
                step="0.01"
                min="0"
                max="1"
                :model-value="slotProps.row.human_score ?? ''"
                style="max-width: 88px"
                @update:model-value="(v: string | number | null) => onScoreChange(slotProps.row, v)"
              />
            </q-td>
          </template>
          <template #body-cell-human_comment="slotProps">
            <q-td :props="slotProps">
              <q-input
                dense
                outlined
                :model-value="slotProps.row.human_comment ?? ''"
                placeholder="评语"
                @update:model-value="(v: string | number | null) => onCommentChange(slotProps.row, String(v ?? ''))"
              />
            </q-td>
          </template>
          <template #body-cell-annotate="slotProps">
            <q-td :props="slotProps">
              <div class="app-registry-cell-actions">
                <q-btn
                  dense
                  flat
                  round
                  icon="save"
                  color="primary"
                  aria-label="保存"
                  :loading="savingId === slotProps.row.id"
                  @click="$emit('annotate', slotProps.row)"
                />
              </div>
            </q-td>
          </template>
        </AppRegistryTable>
        <AppRegistryPagination
          :page="page"
          :page-size="pageSize"
          :page-max="pageMax"
          :total="total"
          :loading="loading"
          label="条用例"
          @update:page="emit('page-change', $event)"
          @update:page-size="emit('page-size-change', $event)"
        />
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn
          v-if="run"
          flat
          color="primary"
          icon="download"
          label="导出 CSV"
          :disable="!total"
          :loading="exporting"
          @click="onExportCsv"
        />
        <q-btn
          v-if="run"
          flat
          color="primary"
          icon="code"
          label="导出 JSON"
          :disable="!total"
          :loading="exporting"
          @click="onExportJson"
        />
        <q-btn flat label="关闭" @click="$emit('update:open', false)" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';
import { getRunResults } from '../../features/evaluation/api';
import { exportEvalRunCsv, exportEvalRunJson } from '../../features/evaluation/exportRunResults';
import type { EvalCaseResult, EvalRun } from '../../features/evaluation/types';
import type { RegistryTableColumn } from '../../features/ui/registryTableColumns';

const props = defineProps<{
  open: boolean;
  runId: string;
  run?: EvalRun | null;
  rows: EvalCaseResult[];
  total: number;
  loading: boolean;
  savingId?: string;
  columns: RegistryTableColumn<EvalCaseResult>[];
  page: number;
  pageSize: number;
}>();

const emit = defineEmits<{
  'update:open': [value: boolean];
  annotate: [row: EvalCaseResult];
  'update-row': [row: EvalCaseResult];
  'page-change': [page: number];
  'page-size-change': [pageSize: number];
}>();

const pageMax = computed(() => Math.max(1, Math.ceil(Math.max(0, props.total) / props.pageSize)));
const exporting = ref(false);

async function loadAllForExport(): Promise<EvalCaseResult[]> {
  if (!props.run) return [];
  const res = await getRunResults(props.run.id, { limit: 5000, offset: 0 });
  return res.items;
}

async function onExportCsv(): Promise<void> {
  if (!props.run) return;
  exporting.value = true;
  try {
    const items = await loadAllForExport();
    exportEvalRunCsv(props.run, items);
  } finally {
    exporting.value = false;
  }
}

async function onExportJson(): Promise<void> {
  if (!props.run) return;
  exporting.value = true;
  try {
    const items = await loadAllForExport();
    exportEvalRunJson(props.run, items);
  } finally {
    exporting.value = false;
  }
}

function passValue(row: EvalCaseResult) {
  if (row.human_pass === true) return 'pass';
  if (row.human_pass === false) return 'fail';
  return 'unset';
}

function onPassChange(row: EvalCaseResult, v: string) {
  const next = { ...row };
  if (v === 'pass') next.human_pass = true;
  else if (v === 'fail') next.human_pass = false;
  else next.human_pass = null;
  emit('update-row', next);
}

function onScoreChange(row: EvalCaseResult, v: string | number | null) {
  const next = { ...row };
  if (v === '' || v === null) next.human_score = null;
  else next.human_score = Number(v);
  emit('update-row', next);
}

function onCommentChange(row: EvalCaseResult, v: string) {
  emit('update-row', { ...row, human_comment: v });
}
</script>
