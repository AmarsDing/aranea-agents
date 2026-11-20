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
    <!-- 卡片视图：构件卡。整卡打开详情（作用域、执行顺序）；主操作是改配置 -->
    <template #item="props">
      <CapCard
        :icon="pluginIcon(props.row)"
        :code="props.row.key"
        :title="props.row.name"
        :description="props.row.description"
        :status="pluginStatus(props.row, t)"
        :facts="pluginFacts(props.row, t)"
        :tags="pluginTags(props.row)"
        @open="props.row.permissions?.can_view && $emit('viewDetail', props.row)"
      >
        <template #switch>
          <q-toggle
            dense
            color="primary"
            :model-value="props.row.enabled"
            :disable="!props.row.permissions?.can_toggle || togglingId === props.row.id"
            :aria-label="props.row.enabled ? t('cap.common.batchDisable') : t('cap.common.batchEnable')"
            @update:model-value="$emit('toggleEnabled', props.row, Boolean($event))"
          />
        </template>
        <!-- 和平台内置机制有重叠的插件：说清它在内置之外多做了什么，免得误以为不开就没有 -->
        <p v-if="overlapKey(props.row.key)" class="cap-note cap-note--info" role="note">
          <q-icon name="info" size="16px" aria-hidden="true" />
          <span class="cap-note__body">{{ t(overlapKey(props.row.key)!) }}</span>
        </p>
        <template #relation>{{ pluginScopeText(props.row, t, agentName) }}</template>
        <template #actions>
          <q-btn
            flat
            dense
            no-caps
            size="sm"
            color="primary"
            icon="tune"
            :label="t('cap.plugins.config')"
            :disable="!props.row.permissions?.can_edit_config"
            @click="$emit('editConfig', props.row)"
          />
          <q-btn flat dense round size="sm" icon="more_horiz" :aria-label="t('cap.common.more')">
            <q-menu anchor="bottom right" self="top right">
              <q-list dense class="cap-menu">
                <q-item
                  v-close-popup
                  clickable
                  :disable="!props.row.permissions?.can_view"
                  @click="$emit('viewDetail', props.row)"
                >
                  <q-item-section avatar><q-icon name="info" size="18px" /></q-item-section>
                  <q-item-section>{{ t('cap.plugins.scopeAndOrder') }}</q-item-section>
                </q-item>
                <q-item clickable :disable="!props.row.permissions?.can_view" :to="pluginRunsTo(props.row)">
                  <q-item-section avatar><q-icon name="history" size="18px" /></q-item-section>
                  <q-item-section>{{ t('cap.common.runs') }}</q-item-section>
                </q-item>
              </q-list>
            </q-menu>
          </q-btn>
        </template>
      </CapCard>
    </template>

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
import CapCard from '../capability/CapCard.vue';
import type { Plugin } from '../../features/plugins/types';
import { pluginFacts, pluginIcon, pluginScopeText, pluginStatus, pluginTags } from './pluginCapCard';
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

const props = withDefaults(
  defineProps<{
    rows: Plugin[];
    loading: boolean;
    togglingId: string;
    shell?: boolean;
    /** Agent id → 名称，卡片上把作用域写成人能看懂的名字 */
    agentOptions?: Array<{ label: string; value: string }>;
  }>(),
  { shell: false, agentOptions: () => [] },
);

const OVERLAP: Record<string, string> = {
  skill_usage_tracker: 'cap.plugins.overlap.skillTracker',
  confirmation_guard: 'cap.plugins.overlap.confirmation',
  permission_guard: 'cap.plugins.overlap.permission',
  cost_guard: 'cap.plugins.overlap.cost',
};
function overlapKey(key: string): string | undefined {
  return OVERLAP[key];
}

function agentName(id: string): string {
  return props.agentOptions.find((o) => o.value === id)?.label ?? id;
}

defineEmits<{
  toggleEnabled: [row: Plugin, value: boolean];
  viewDetail: [row: Plugin];
  editConfig: [row: Plugin];
}>();

const { t } = useI18n();

const tableColumns = computed(() => createPluginTableColumns(t));
</script>
