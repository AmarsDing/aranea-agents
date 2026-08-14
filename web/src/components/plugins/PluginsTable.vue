<template>
  <AppRegistryTable
    :shell="shell"
    table-class="plugins-table"
    :rows="rows"
    :columns="tableColumns"
    row-key="id"
    :loading="loading"
    hide-pagination
    :pagination="{ rowsPerPage: 0 }"
    column-persist-key="plugins-table"
  >
    <template #body-cell-name="props">
      <q-td :props="props">
        <AppRegistryHoverTip :text="props.row.description" :empty-label="t('pluginsPage.noDescription')">
          <div class="plugins-table__name-hit min-width-0">
            <div class="app-registry-cell-primary ellipsis">{{ props.row.name }}</div>
            <div class="app-registry-cell-sub ellipsis">{{ props.row.key }}</div>
          </div>
        </AppRegistryHoverTip>
      </q-td>
    </template>

    <template #body-cell-category="props">
      <q-td :props="props">
        <div class="app-registry-chip-wrap plugins-table__tag-wrap">
          <span class="plugin-tag plugin-tag--category">{{ props.row.category }}</span>
          <span class="plugin-tag" :class="riskTagClass(props.row.risk_level)">{{ props.row.risk_level }}</span>
        </div>
      </q-td>
    </template>

    <template #body-cell-callbacks="props">
      <q-td :props="props">
        <AppRegistryHoverTip
          :text="formatCallbacksSummary(props.row.callback_points)"
          :empty-label="t('pluginsPage.noCallbacks')"
        >
          <div v-if="props.row.callback_points?.length" class="app-registry-chip-wrap plugins-table__callback-chips">
            <span
              v-for="point in visibleCallbackPoints(props.row.callback_points)"
              :key="point"
              class="plugin-tag plugin-tag--callback"
            >
              {{ point }}
            </span>
            <span v-if="hiddenCallbackCount(props.row.callback_points)" class="plugin-tag plugin-tag--more">
              +{{ hiddenCallbackCount(props.row.callback_points) }}
            </span>
          </div>
          <span v-else class="text-caption text-grey-7">—</span>
        </AppRegistryHoverTip>
      </q-td>
    </template>

    <template #body-cell-stats="props">
      <q-td :props="props">
        <div class="plugin-stats-cell min-width-0">
          <div class="app-registry-cell-primary">
            {{ t('pluginsPage.invokeTimes', { count: props.row.invoke_count }) }}
          </div>
          <div class="plugin-stats-cell__meta">
            <span class="plugin-status-dot" :class="`plugin-status-dot--${lastStatusTone(props.row)}`" />
            <span class="app-registry-cell-sub ellipsis">{{ lastStatusLabel(props.row, t) }}</span>
          </div>
          <div v-if="props.row.error_count || props.row.block_count" class="app-registry-cell-sub ellipsis">
            {{ t('pluginsPage.statsBlockError', { block: props.row.block_count, error: props.row.error_count }) }}
          </div>
        </div>
      </q-td>
    </template>

    <template #body-cell-enabled="props">
      <q-td :props="props" class="plugins-table__toggle-cell">
        <q-toggle
          :model-value="props.row.enabled"
          color="primary"
          dense
          :disable="!props.row.permissions?.can_toggle || togglingId === props.row.id"
          @update:model-value="$emit('toggleEnabled', props.row, Boolean($event))"
        />
      </q-td>
    </template>

    <template #body-cell-scope="props">
      <q-td :props="props">
        <span class="plugin-tag plugin-tag--scope ellipsis" :title="scopeTooltip(props.row.scope, t)">
          {{ scopeLabel(props.row.scope) }}
        </span>
      </q-td>
    </template>

    <template #body-cell-actions="props">
      <q-td :props="props" class="plugins-table__actions-cell">
        <div class="app-registry-cell-actions">
          <q-btn
            v-if="props.row.permissions?.can_view"
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="secondary"
            icon="history"
            :aria-label="t('plugins.actionViewRuns')"
            :to="pluginRunsTo(props.row)"
          >
            <q-tooltip>{{ t('plugins.actionViewRuns') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="primary"
            icon="visibility"
            :aria-label="t('plugins.actionViewDetail')"
            :disable="!props.row.permissions?.can_view"
            @click="$emit('viewDetail', props.row)"
          >
            <q-tooltip>{{ t('plugins.actionViewDetail') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="primary"
            icon="settings"
            :aria-label="t('plugins.actionEditConfig')"
            :disable="!props.row.permissions?.can_edit_config"
            @click="$emit('editConfig', props.row)"
          >
            <q-tooltip>{{ t('plugins.actionEditConfig') }}</q-tooltip>
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
import type { Plugin } from '../../features/plugins/types';
import {
  createPluginTableColumns,
  formatCallbacksSummary,
  hiddenCallbackCount,
  lastStatusLabel,
  lastStatusTone,
  pluginRunsTo,
  riskTagClass,
  scopeLabel,
  scopeTooltip,
  visibleCallbackPoints,
} from './pluginUi';

withDefaults(
  defineProps<{
    rows: Plugin[];
    loading: boolean;
    togglingId: string;
    shell?: boolean;
  }>(),
  { shell: false },
);

defineEmits<{
  toggleEnabled: [row: Plugin, value: boolean];
  viewDetail: [row: Plugin];
  editConfig: [row: Plugin];
}>();

const { t } = useI18n();

const tableColumns = computed(() => createPluginTableColumns(t));
</script>
