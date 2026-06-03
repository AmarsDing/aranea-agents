<template>
  <AppRegistryTable
    :shell="shell"
    table-class="hooks-data-table"
    :rows="rows"
    :columns="tableColumns"
    row-key="id"
    :loading="loading"
    hide-pagination
    :pagination="{ rowsPerPage: 0 }"
    :column-persist-key="columnPersistKey"
  >
    <template #body-cell-name="props">
      <q-td :props="props">
        <AppRegistryHoverTip :text="props.row.description" empty-label="暂无说明">
          <div class="min-width-0">
            <div class="app-registry-cell-primary ellipsis">{{ props.row.name }}</div>
            <div class="app-registry-cell-sub ellipsis">{{ props.row.key }}</div>
          </div>
        </AppRegistryHoverTip>
      </q-td>
    </template>

    <template #body-cell-rule="props">
      <q-td :props="props">
        <AppRegistryHoverTip :text="hookConditionHint(props.row, t)" :empty-label="t('hooksPage.noDescription')">
          <div class="app-registry-chip-wrap hooks-data-table__rule-tags">
            <span class="hook-tag hook-tag--point">{{ hookRuleOf(props.row).callback_point }}</span>
            <span :class="actionTagClass(hookRuleOf(props.row).action.type)">
              {{ actionTypeLabel(hookRuleOf(props.row).action.type, t) }}
            </span>
          </div>
        </AppRegistryHoverTip>
      </q-td>
    </template>

    <template #body-cell-sort="props">
      <q-td :props="props">
        <span class="hooks-data-table__sort">{{ props.row.sort_order }}</span>
      </q-td>
    </template>

    <template #body-cell-enabled="props">
      <q-td :props="props" class="hooks-data-table__toggle-cell">
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
      <q-td :props="props" class="hooks-data-table__actions-cell">
        <div class="app-registry-cell-actions">
          <q-btn
            v-if="variant === 'page'"
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="secondary"
            icon="history"
            :to="hookRunsTo(props.row)"
          >
            <q-tooltip>{{ t('hooksPage.tooltipViewRuns') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="primary"
            icon="edit"
            @click="$emit('edit', props.row)"
          >
            <q-tooltip>{{ t('hooksPage.tooltipEdit') }}</q-tooltip>
          </q-btn>
          <q-btn
            v-if="variant === 'page'"
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="negative"
            icon="delete"
            @click="$emit('remove', props.row)"
          >
            <q-tooltip>{{ t('hooksPage.tooltipDelete') }}</q-tooltip>
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
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import type { HookRow } from '../../features/hooks/types';
import { actionTagClass, actionTypeLabel } from './callbackEditorUi';
import {
  createHooksAgentTableColumns,
  createHooksTableColumns,
  hookConditionHint,
  hookRuleOf,
  hookRunsTo,
} from './hookTableUi';

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    rows: HookRow[];
    loading: boolean;
    togglingId?: string;
    variant?: 'page' | 'agent';
    shell?: boolean;
  }>(),
  {
    togglingId: '',
    variant: 'page',
    shell: false,
  },
);

defineEmits<{
  toggleEnabled: [row: HookRow, value: boolean];
  edit: [row: HookRow];
  remove: [row: HookRow];
}>();

const tableColumns = computed(() =>
  props.variant === 'agent' ? createHooksAgentTableColumns(t) : createHooksTableColumns(t),
);

const columnPersistKey = computed(() => (props.variant === 'agent' ? 'hooks-agent-table' : 'hooks-table'));
</script>
