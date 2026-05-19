<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card style="min-width: 560px; max-width: 96vw">
      <q-card-section class="text-h6">用例结果 · {{ runId }}</q-card-section>
      <q-card-section>
        <q-table flat :rows="rows" :columns="columns" row-key="id" :loading="loading" :pagination="{ rowsPerPage: 10 }" />
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat label="关闭" @click="$emit('update:open', false)" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import type { EvalCaseResult } from "../../features/evaluation/types";

defineProps<{
  open: boolean;
  runId: string;
  rows: EvalCaseResult[];
  loading: boolean;
  columns: { name: string; label: string; field: string | ((row: EvalCaseResult) => unknown); align: "left" | "center" | "right" }[];
}>();
defineEmits<{ "update:open": [value: boolean] }>();
</script>
