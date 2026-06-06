<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
      <q-card-section class="text-h6">用例结果 · {{ runId }}</q-card-section>
      <q-card-section class="app-dialog-body q-pt-none">
        <AppRegistryTable
          :shell="false"
          :data-shell="true"
          :rows="pagedRows"
          :columns="columns"
          row-key="id"
          :loading="loading"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
        >
          <template #body-cell-case_id="props">
            <q-td :props="props">
              <AppRegistryHoverTip :text="props.row.error_message">
                <span class="app-registry-cell-primary ellipsis">{{ props.row.case_id }}</span>
              </AppRegistryHoverTip>
            </q-td>
          </template>
          <template #body-cell-human_pass="props">
            <q-td :props="props">
              <q-btn-toggle
                :model-value="passValue(props.row)"
                toggle-color="primary"
                dense
                unelevated
                :options="[
                  { label: '-', value: 'unset' },
                  { label: 'Pass', value: 'pass' },
                  { label: 'Fail', value: 'fail' },
                ]"
                @update:model-value="(v: string) => onPassChange(props.row, v)"
              />
            </q-td>
          </template>
          <template #body-cell-human_score="props">
            <q-td :props="props">
              <q-input
                dense
                outlined
                type="number"
                step="0.01"
                min="0"
                max="1"
                :model-value="props.row.human_score ?? ''"
                style="max-width: 88px"
                @update:model-value="(v: string | number | null) => onScoreChange(props.row, v)"
              />
            </q-td>
          </template>
          <template #body-cell-human_comment="props">
            <q-td :props="props">
              <q-input
                dense
                outlined
                :model-value="props.row.human_comment ?? ''"
                placeholder="评语"
                @update:model-value="(v: string | number | null) => onCommentChange(props.row, String(v ?? ''))"
              />
            </q-td>
          </template>
          <template #body-cell-annotate="props">
            <q-td :props="props">
              <div class="app-registry-cell-actions">
                <q-btn
                  dense
                  flat
                  round
                  icon="save"
                  color="primary"
                  aria-label="保存"
                  :loading="savingId === props.row.id"
                  @click="$emit('annotate', props.row)"
                />
              </div>
            </q-td>
          </template>
        </AppRegistryTable>
        <AppRegistryPagination
          v-model:page="page"
          v-model:page-size="pageSize"
          :page-max="pageMax"
          :total="rows.length"
          :loading="loading"
          label="条用例"
        />
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn
          v-if="run"
          flat
          color="primary"
          icon="download"
          label="导出 CSV"
          :disable="!rows.length"
          @click="onExportCsv"
        />
        <q-btn
          v-if="run"
          flat
          color="primary"
          icon="code"
          label="导出 JSON"
          :disable="!rows.length"
          @click="onExportJson"
        />
        <q-btn flat label="关闭" @click="$emit('update:open', false)" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';
import { exportEvalRunCsv, exportEvalRunJson } from '../../features/evaluation/exportRunResults';
import type { EvalCaseResult, EvalRun } from '../../features/evaluation/types';
import type { RegistryTableColumn } from '../../features/ui/registryTableColumns';

const props = defineProps<{
  open: boolean;
  runId: string;
  run?: EvalRun | null;
  rows: EvalCaseResult[];
  loading: boolean;
  savingId?: string;
  columns: RegistryTableColumn<EvalCaseResult>[];
}>();

const page = ref(1);
const pageSize = ref(10);
const pageMax = computed(() => Math.max(1, Math.ceil(props.rows.length / pageSize.value)));
const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return props.rows.slice(start, start + pageSize.value);
});

watch(
  () => props.runId,
  () => {
    page.value = 1;
  },
);

watch(
  () => props.rows.length,
  () => {
    if (page.value > pageMax.value) page.value = pageMax.value;
  },
);

function onExportCsv(): void {
  if (!props.run) return;
  exportEvalRunCsv(props.run, props.rows);
}

function onExportJson(): void {
  if (!props.run) return;
  exportEvalRunJson(props.run, props.rows);
}

const emit = defineEmits<{
  'update:open': [value: boolean];
  annotate: [row: EvalCaseResult];
  'update-row': [row: EvalCaseResult];
}>();

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
