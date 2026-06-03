<template>
  <AppRegistryTable
    :shell="shell"
    table-class="webhooks-data-table"
    :rows="rows"
    :columns="tableColumns"
    row-key="id"
    :loading="loading"
    hide-pagination
    :pagination="{ rowsPerPage: 0 }"
    column-persist-key="webhooks-table"
  >
    <template #body-cell-name="props">
      <q-td :props="props">
        <div class="min-width-0">
          <div class="app-registry-cell-primary ellipsis">{{ props.row.name }}</div>
          <div class="app-registry-cell-sub ellipsis">{{ props.row.url }}</div>
        </div>
      </q-td>
    </template>

    <template #body-cell-events="props">
      <q-td :props="props">
        <div class="app-registry-chip-wrap webhooks-data-table__event-tags">
          <span
            v-for="evt in parseEventTypes(props.row.event_types_json)"
            :key="evt"
            class="webhook-tag webhook-tag--event"
            >{{ evt }}</span
          >
          <span v-if="parseEventTypes(props.row.event_types_json).length === 0" class="app-registry-cell-sub">—</span>
        </div>
      </q-td>
    </template>

    <template #body-cell-enabled="props">
      <q-td :props="props" class="webhooks-data-table__toggle-cell">
        <q-toggle
          :model-value="props.row.enabled"
          color="primary"
          dense
          :disable="togglingId === props.row.id"
          @update:model-value="$emit('toggleEnabled', props.row, Boolean($event))"
        />
      </q-td>
    </template>

    <template #body-cell-actions="props">
      <q-td :props="props" class="webhooks-data-table__actions-cell">
        <div class="app-registry-cell-actions">
          <q-btn
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="primary"
            icon="edit"
            @click="$emit('edit', props.row)"
          >
            <q-tooltip>{{ t('webhooksPage.tooltipEdit') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="negative"
            icon="delete"
            @click="$emit('remove', props.row)"
          >
            <q-tooltip>{{ t('webhooksPage.tooltipDelete') }}</q-tooltip>
          </q-btn>
        </div>
      </q-td>
    </template>
  </AppRegistryTable>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import type { WebhookRow } from '../../features/webhooks/types';
import { createWebhookColumns } from './webhookTableUi';

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    rows: WebhookRow[];
    loading: boolean;
    togglingId?: string;
    shell?: boolean;
  }>(),
  {
    togglingId: '',
    shell: false,
  },
);

defineEmits<{
  toggleEnabled: [row: WebhookRow, value: boolean];
  edit: [row: WebhookRow];
  remove: [row: WebhookRow];
}>();

const tableColumns = computed(() => createWebhookColumns(t));

function parseEventTypes(json: string): string[] {
  if (!json?.trim()) return [];
  try {
    const parsed = JSON.parse(json);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}
</script>
