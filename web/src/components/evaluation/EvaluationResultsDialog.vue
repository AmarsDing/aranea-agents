<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card style="min-width: 720px; max-width: 96vw">
      <q-card-section class="text-h6">用例结果 · {{ runId }}</q-card-section>
      <q-card-section>
        <q-table flat :rows="rows" :columns="columns" row-key="id" :loading="loading" :pagination="{ rowsPerPage: 10 }">
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
                  { label: 'Fail', value: 'fail' }
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
              <q-btn dense flat color="primary" label="保存" :loading="savingId === props.row.id" @click="$emit('annotate', props.row)" />
            </q-td>
          </template>
        </q-table>
      </q-card-section>
      <q-card-actions align="right">
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
import { exportEvalRunCsv, exportEvalRunJson } from "../../features/evaluation/exportRunResults";
import type { EvalCaseResult, EvalRun } from "../../features/evaluation/types";

const props = defineProps<{
  open: boolean;
  runId: string;
  run?: EvalRun | null;
  rows: EvalCaseResult[];
  loading: boolean;
  savingId?: string;
  columns: { name: string; label: string; field: string | ((row: EvalCaseResult) => unknown); align: "left" | "center" | "right" }[];
}>();

function onExportCsv(): void {
  if (!props.run) return;
  exportEvalRunCsv(props.run, props.rows);
}

function onExportJson(): void {
  if (!props.run) return;
  exportEvalRunJson(props.run, props.rows);
}

const emit = defineEmits<{
  "update:open": [value: boolean];
  annotate: [row: EvalCaseResult];
  "update-row": [row: EvalCaseResult];
}>();

function passValue(row: EvalCaseResult) {
  if (row.human_pass === true) return "pass";
  if (row.human_pass === false) return "fail";
  return "unset";
}

function onPassChange(row: EvalCaseResult, v: string) {
  const next = { ...row };
  if (v === "pass") next.human_pass = true;
  else if (v === "fail") next.human_pass = false;
  else next.human_pass = null;
  emit("update-row", next);
}

function onScoreChange(row: EvalCaseResult, v: string | number | null) {
  const next = { ...row };
  if (v === "" || v === null) next.human_score = null;
  else next.human_score = Number(v);
  emit("update-row", next);
}

function onCommentChange(row: EvalCaseResult, v: string) {
  emit("update-row", { ...row, human_comment: v });
}
</script>
