<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
      <q-card-section class="text-h6">{{ $t('evaluationPage.versionsTitle') }}</q-card-section>
      <q-card-section class="app-dialog-body q-pt-none">
        <q-table
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
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat no-caps :label="$t('common.close')" @click="$emit('update:open', false)" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import type { QTableColumn } from 'quasar';
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
