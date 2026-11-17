<template>
  <AppDialog
    :model-value="open"
    :title="$t('evaluationPage.versionsTitle')"
    size="md"
    persistent
    @update:model-value="$emit('update:open', $event)"
  >
    <AppRegistryTable
      :shell="false"
      :resizable="false"
      flat
      dense
      row-key="id"
      :rows="rows"
      :columns="columns"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
    >
      <template #body-cell-actions="slotProps">
        <q-td :props="slotProps">
          <q-btn
            flat
            dense
            no-caps
            color="primary"
            icon="play_arrow"
            :label="$t('evaluationPage.versionRun')"
            @click="$emit('run', slotProps.row)"
          />
        </q-td>
      </template>
    </AppRegistryTable>
    <template #actions>
      <q-btn flat no-caps :label="$t('common.close')" @click="$emit('update:open', false)" />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import type { QTableColumn } from 'quasar';
import AppDialog from '../common/AppDialog.vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import type { EvalDatasetVersion } from '../../features/evaluation/types';

defineProps<{
  open: boolean;
  rows: EvalDatasetVersion[];
  columns: QTableColumn<EvalDatasetVersion>[];
  loading: boolean;
}>();
defineEmits<{
  'update:open': [value: boolean];
  run: [value: EvalDatasetVersion];
}>();
</script>
