<template>
  <q-table flat :rows="rows" :columns="columns" row-key="id" :loading="loading" :pagination="{ rowsPerPage: 15 }">
    <template #body-cell-status="props">
      <q-td :props="props">
        <q-chip dense :color="statusColor(props.row.status)" text-color="white" size="sm">{{ props.row.status }}</q-chip>
      </q-td>
    </template>
  </q-table>
</template>

<script setup lang="ts">
import type { A2AAuditEntry } from "../../features/a2a/types";

defineProps<{
  rows: A2AAuditEntry[];
  loading: boolean;
  columns: { name: string; label: string; field: string; align: "left" | "right" | "center" }[];
  statusColor: (status: string) => string;
}>();
</script>
