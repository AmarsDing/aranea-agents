<template>
  <AppDialog
    :model-value="open"
    :title="$t('evaluationPage.casesTitle')"
    size="lg"
    persistent
    @update:model-value="$emit('update:open', $event)"
  >
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
      <template #body-cell-input="slotProps">
        <q-td :props="slotProps">
          <span class="app-registry-cell-sub ellipsis" :title="slotProps.row.input">{{ slotProps.row.input }}</span>
        </q-td>
      </template>
      <template #body-cell-expected_output="slotProps">
        <q-td :props="slotProps">
          <span class="app-registry-cell-sub ellipsis" :title="slotProps.row.expected_output">{{
            slotProps.row.expected_output
          }}</span>
        </q-td>
      </template>
      <template #body-cell-actions="slotProps">
        <q-td :props="slotProps">
          <div class="app-registry-cell-actions">
            <q-btn
              flat
              dense
              round
              icon="edit"
              color="primary"
              :aria-label="$t('evaluationPage.caseEdit')"
              @click="$emit('edit', slotProps.row)"
            />
            <q-btn
              flat
              dense
              round
              icon="delete"
              color="negative"
              :aria-label="$t('evaluationPage.caseDelete')"
              @click="$emit('remove', slotProps.row)"
            />
          </div>
        </q-td>
      </template>
    </AppRegistryTable>
    <div v-if="editId" class="q-gutter-sm q-mt-md">
      <q-input
        :model-value="editInput"
        class="app-field-long"
        dense
        outlined
        type="textarea"
        autogrow
        :label="$t('evaluationPage.caseInput')"
        @update:model-value="$emit('update:editInput', String($event ?? ''))"
      />
      <q-input
        :model-value="editExpected"
        class="app-field-long"
        dense
        outlined
        type="textarea"
        autogrow
        :label="$t('evaluationPage.caseExpected')"
        @update:model-value="$emit('update:editExpected', String($event ?? ''))"
      />
      <div class="app-actions-bar app-actions-bar--end">
        <q-btn flat no-caps :label="$t('evaluationPage.caseEditCancel')" @click="$emit('cancel-edit')" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :label="$t('evaluationPage.caseSave')"
          :loading="saving"
          :disable="!editInput.trim()"
          @click="$emit('save')"
        />
      </div>
    </div>
    <template #actions>
      <q-btn flat no-caps :label="$t('evaluationPage.casesClose')" @click="$emit('update:open', false)" />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import AppDialog from '../common/AppDialog.vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import type { EvalCase } from '../../features/evaluation/types';
import type { QTableColumn } from 'quasar';

defineProps<{
  open: boolean;
  rows: EvalCase[];
  columns: QTableColumn<EvalCase>[];
  loading: boolean;
  saving: boolean;
  editId: string;
  editInput: string;
  editExpected: string;
}>();

defineEmits<{
  'update:open': [value: boolean];
  'update:editInput': [value: string];
  'update:editExpected': [value: string];
  edit: [row: EvalCase];
  remove: [row: EvalCase];
  save: [];
  'cancel-edit': [];
}>();
</script>
