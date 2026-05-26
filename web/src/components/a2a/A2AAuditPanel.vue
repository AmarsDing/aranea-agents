<template>
  <div>
    <AppRegistryTable
      :rows="pagedRows"
      :columns="columns"
      row-key="id"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
    >
      <template #body-cell-status="props">
        <q-td :props="props">
          <q-chip dense :color="statusColor(props.row.status)" text-color="white" size="sm">{{ props.row.status }}</q-chip>
        </q-td>
      </template>
    </AppRegistryTable>
    <AppRegistryPagination
      v-model:page="page"
      v-model:page-size="pageSize"
      :page-max="pageMax"
      :total="rows.length"
      :loading="loading"
      label="条审计"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { A2AAuditEntry } from "../../features/a2a/types";
import type { RegistryTableColumn } from "../../features/ui/registryTableColumns";
import AppRegistryTable from "../layout/AppRegistryTable.vue";
import AppRegistryPagination from "../layout/AppRegistryPagination.vue";

const props = defineProps<{
  rows: A2AAuditEntry[];
  loading: boolean;
  columns: RegistryTableColumn<A2AAuditEntry>[];
  statusColor: (status: string) => string;
}>();

const page = ref(1);
const pageSize = ref(15);
const pageMax = computed(() => Math.max(1, Math.ceil(props.rows.length / pageSize.value)));
const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return props.rows.slice(start, start + pageSize.value);
});

watch(
  () => props.rows.length,
  () => {
    if (page.value > pageMax.value) page.value = pageMax.value;
  }
);
</script>
