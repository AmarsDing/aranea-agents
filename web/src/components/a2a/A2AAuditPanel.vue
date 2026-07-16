<template>
  <div>
    <AppRegistryTable
      :rows="rows"
      :columns="columns"
      row-key="id"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
    >
      <template #body-cell-status="slotProps">
        <q-td :props="slotProps">
          <q-chip dense :color="statusColor(slotProps.row.status)" text-color="white" size="sm">{{
            slotProps.row.status
          }}</q-chip>
        </q-td>
      </template>
    </AppRegistryTable>
    <AppRegistryPagination
      :page="page"
      :page-size="pageSize"
      :page-max="pageMax"
      :total="total"
      :loading="loading"
      label="条审计"
      @update:page="emit('page-change', $event)"
      @update:page-size="emit('page-size-change', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { A2AAuditEntry } from '../../features/a2a/types';
import type { RegistryTableColumn } from '../../features/ui/registryTableColumns';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';

const props = defineProps<{
  rows: A2AAuditEntry[];
  total: number;
  loading: boolean;
  columns: RegistryTableColumn<A2AAuditEntry>[];
  statusColor: (status: string) => string;
  page: number;
  pageSize: number;
}>();

const emit = defineEmits<{
  'page-change': [page: number];
  'page-size-change': [pageSize: number];
}>();

const pageMax = computed(() => Math.max(1, Math.ceil(Math.max(0, props.total) / props.pageSize)));
</script>
