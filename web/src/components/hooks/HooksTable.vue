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
    <template #no-data>
      <div class="full-width row flex-center q-pa-lg text-grey-6">
        {{ emptyText }}
      </div>
    </template>

    <template #body-cell-name="slotProps">
      <q-td :props="slotProps">
        <AppRegistryHoverTip :text="slotProps.row.description" :empty-label="t('hooksPage.noDescription')">
          <div class="min-width-0">
            <div class="app-registry-cell-primary ellipsis">{{ slotProps.row.name }}</div>
            <div class="app-registry-cell-sub ellipsis">{{ slotProps.row.key }}</div>
          </div>
        </AppRegistryHoverTip>
      </q-td>
    </template>

    <template #body-cell-rule="slotProps">
      <q-td :props="slotProps">
        <AppRegistryHoverTip :text="hookConditionHint(slotProps.row, t)" :empty-label="t('hooksPage.noDescription')">
          <div class="app-registry-chip-wrap hooks-data-table__rule-tags">
            <span class="hook-tag hook-tag--point">{{
              callbackPointLabel(hookRuleOf(slotProps.row).callback_point, t)
            }}</span>
            <span :class="actionTagClass(hookRuleOf(slotProps.row).action.type)">
              {{ actionTypeLabel(hookRuleOf(slotProps.row).action.type, t) }}
            </span>
          </div>
        </AppRegistryHoverTip>
      </q-td>
    </template>

    <template #body-cell-sort="slotProps">
      <q-td :props="slotProps">
        <span class="hooks-data-table__sort">{{ slotProps.row.sort_order }}</span>
      </q-td>
    </template>

    <template #body-cell-enabled="slotProps">
      <q-td :props="slotProps" class="hooks-data-table__toggle-cell">
        <q-toggle
          :model-value="slotProps.row.enabled"
          color="primary"
          dense
          :disable="togglingId === slotProps.row.id"
          @update:model-value="$emit('toggleEnabled', slotProps.row, Boolean($event))"
        />
      </q-td>
    </template>

    <template #body-cell-actions="slotProps">
      <q-td :props="slotProps" class="hooks-data-table__actions-cell">
        <div class="app-registry-cell-actions">
          <q-btn
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="secondary"
            icon="history"
            :to="hookRunsTo(slotProps.row)"
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
            @click="$emit('edit', slotProps.row)"
          >
            <q-tooltip>{{ t('hooksPage.tooltipEdit') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="negative"
            icon="delete"
            @click="$emit('remove', slotProps.row)"
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
import { callbackPointLabel } from '../../features/callback/constants';
import { actionTagClass, actionTypeLabel } from './callbackEditorUi';
import {
  createHooksAgentTableColumns,
  createHooksReadonlyTableColumns,
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
    /** 只读模式：仅名称 + 规则两列，无操作（用于全局规则分组展示）。 */
    readonly?: boolean;
  }>(),
  {
    togglingId: '',
    variant: 'page',
    shell: false,
    readonly: false,
  },
);

defineEmits<{
  toggleEnabled: [row: HookRow, value: boolean];
  edit: [row: HookRow];
  remove: [row: HookRow];
}>();

const tableColumns = computed(() => {
  if (props.readonly) return createHooksReadonlyTableColumns(t);
  return props.variant === 'agent' ? createHooksAgentTableColumns(t) : createHooksTableColumns(t);
});

const emptyText = computed(() =>
  props.variant === 'agent' && !props.readonly ? t('hooksPage.agentPanel.emptyScoped') : t('hooksPage.emptyList'),
);

const columnPersistKey = computed(() => {
  if (props.readonly) return 'hooks-readonly-table';
  return props.variant === 'agent' ? 'hooks-agent-table' : 'hooks-table';
});
</script>
