<template>
  <q-page class="app-standard-page cap-page plugins-page">
    <!-- 运行记录在子页签里；插件是平台内置的一组，不能新建，这里只留刷新 -->
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :title="t('cap.common.refresh')"
        :aria-label="t('cap.common.refresh')"
        :loading="loading"
        @click="() => loadRows()"
      />
    </Teleport>

    <CapStats :stats="stats" :label="t('cap.common.statsLabel')" :active="activeStat" @pick="onPickStat" />

    <!-- 安全类插件没开时如实提醒：只有启用的插件才会挂到 Agent 的运行链路上 -->
    <p v-if="securityOff.length" class="cap-note" role="note">
      <q-icon name="shield" size="16px" aria-hidden="true" />
      <span class="cap-note__body">
        {{ t('cap.plugins.securityOffLead') }}
        <span v-for="gap in securityOff" :key="gap" class="cap-note__item">{{ gap }}</span>
        {{ t('cap.plugins.securityOffBase') }}
      </span>
    </p>

    <div class="cap-bar" role="search">
      <q-input
        v-model="search"
        class="cap-bar__search"
        dense
        outlined
        clearable
        debounce="250"
        :placeholder="t('cap.plugins.searchPlaceholder')"
        :aria-label="t('cap.plugins.searchPlaceholder')"
      >
        <template #prepend><q-icon name="search" size="18px" /></template>
      </q-input>
      <div class="cap-seg" role="group" :aria-label="t('cap.common.statusFilter')">
        <button
          v-for="opt in statusSegments"
          :key="String(opt.value)"
          type="button"
          :aria-pressed="enabled === opt.value"
          @click="enabled = opt.value"
        >
          {{ opt.label }}
        </button>
      </div>
      <q-select
        v-model="category"
        dense
        outlined
        clearable
        emit-value
        map-options
        options-dense
        :label="t('cap.plugins.category')"
        :options="categoryOptions"
      />
      <q-select
        v-model="callbackPoint"
        dense
        outlined
        clearable
        emit-value
        map-options
        options-dense
        :label="t('cap.plugins.hookPoint')"
        :options="callbackPointOptions"
      />
      <span class="cap-bar__spacer" />
      <q-btn
        v-if="filtersActive"
        flat
        dense
        no-caps
        icon="filter_alt_off"
        :label="t('cap.common.reset')"
        @click="resetFilters"
      />
    </div>

    <AppStatePanel v-if="pageState === 'loading'" variant="loading" :rows="5" />
    <AppStatePanel
      v-else-if="pageState === 'error'"
      variant="error"
      :title="error"
      :action-label="t('pluginsPage.retry')"
      action-icon="refresh"
      @action="() => loadRows()"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" variant="forbidden" />
    <AppStatePanel
      v-else-if="pageState === 'empty'"
      variant="empty"
      icon="extension"
      :title="t('cap.plugins.emptyTitle')"
      :action-label="filtersActive ? t('cap.common.reset') : ''"
      action-icon="filter_alt_off"
      @action="resetFilters"
    />

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
        variant="partial"
        dense
        :description="error"
        :action-label="t('pluginsPage.retry')"
        action-icon="refresh"
        @action="() => loadRows()"
      />
      <PluginsTable
        shell
        :rows="rows"
        :loading="loading"
        :toggling-id="togglingId"
        :agent-options="agentOptions"
        @toggle-enabled="toggleEnabled"
        @view-detail="openDetail"
        @edit-config="openConfig"
      />

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="total"
        :loading="loading"
        :label="t('pluginsPage.paginationLabel')"
      />
    </template>

    <PluginDetailDialog
      v-model:open="detailOpen"
      v-model:scope-mode="scopeMode"
      v-model:scope-agent-id="scopeAgentId"
      :target="detailTarget"
      :agent-options="agentOptions"
      :saving-scope="savingScope"
      :bumping-sort="bumpingSort"
      @bump-sort="(delta) => detailTarget && bumpSort(detailTarget, delta)"
      @save-scope="saveScope"
    />

    <PluginConfigDialog
      v-model:open="configOpen"
      v-model:config-text="configText"
      v-model:mode="configMode"
      :target="configTarget"
      :config-error="configError"
      :saving="savingConfig"
      @save="saveConfig"
      @validation-error="onSchemaValidationError"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import CapStats from '../components/capability/CapStats.vue';
import { formatCount, type CapTone } from '../components/capability/capTypes';
import PluginConfigDialog from '../components/plugins/PluginConfigDialog.vue';
import PluginDetailDialog from '../components/plugins/PluginDetailDialog.vue';
import PluginsTable from '../components/plugins/PluginsTable.vue';
import { SECURITY_CATEGORY } from '../components/plugins/pluginCapCard';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import { usePluginsPage } from '../features/plugins/usePluginsPage';
import { resolvePageState } from './pageState';

const { t } = useI18n();

const {
  rows,
  total,
  page,
  pageSize,
  pageMax,
  search,
  category,
  enabled,
  callbackPoint,
  loading,
  error,
  forbidden,
  togglingId,
  detailOpen,
  detailTarget,
  scopeMode,
  scopeAgentId,
  agentOptions,
  savingScope,
  bumpingSort,
  configOpen,
  configTarget,
  configText,
  configMode,
  savingConfig,
  configError,
  categoryOptions,
  callbackPointOptions,
  loadRows,
  resetFilters,
  toggleEnabled,
  openDetail,
  openConfig,
  saveConfig,
  bumpSort,
  saveScope,
  onSchemaValidationError,
} = usePluginsPage();

const statusSegments = computed(() => [
  { value: null, label: t('cap.common.all') },
  { value: true, label: t('cap.common.enabled') },
  { value: false, label: t('cap.common.disabled') },
]);

const filtersActive = computed(
  () => Boolean(search.value || category.value || callbackPoint.value) || enabled.value !== null,
);

// shortcut: 插件是内置的一组（目前 9 个），一页放得下，关键数按已读到的行算；内置插件超过一页时改由后端给汇总
const stats = computed(() => {
  const list = rows.value;
  const on = list.filter((p) => p.enabled).length;
  const security = list.filter((p) => p.category === SECURITY_CATEGORY);
  const securityOn = security.filter((p) => p.enabled).length;
  const runs = list.reduce((n, p) => n + (p.invoke_count || 0), 0);
  const bad = list.reduce((n, p) => n + (p.error_count || 0), 0);
  const blocked = list.reduce((n, p) => n + (p.block_count || 0), 0);
  return [
    {
      key: 'total',
      label: t(filtersActive.value ? 'cap.plugins.stats.matched' : 'cap.plugins.stats.total'),
      value: total.value,
    },
    { key: 'enabled', label: t('cap.plugins.stats.enabled'), value: `${on} / ${list.length}`, clickable: true },
    {
      key: 'security',
      label: t('cap.plugins.stats.security'),
      value: `${securityOn} / ${security.length}`,
      tone: (security.length && securityOn < security.length ? 'watch' : 'ok') as CapTone,
    },
    { key: 'runs', label: t('cap.plugins.stats.runs'), value: formatCount(runs) },
    {
      key: 'blocked',
      label: t('cap.plugins.stats.blockedError'),
      value: `${blocked} / ${bad}`,
      tone: (bad > 0 ? 'alert' : blocked > 0 ? 'watch' : undefined) as CapTone | undefined,
    },
  ];
});

// 没开的安全插件各自少了哪层保护。工具上的「需人工确认」和各 Agent 的工具权限不靠插件，照常生效，这里不算缺口
const SECURITY_GAP: Record<string, string> = {
  sensitive_data_mask: 'cap.plugins.gap.mask',
  output_policy: 'cap.plugins.gap.output',
  confirmation_guard: 'cap.plugins.gap.confirm',
  permission_guard: 'cap.plugins.gap.permission',
};
const securityOff = computed(() =>
  rows.value
    .filter((p) => p.category === SECURITY_CATEGORY && !p.enabled)
    .map((p) => (SECURITY_GAP[p.key] ? t(SECURITY_GAP[p.key]) : p.name)),
);

const activeStat = computed(() => (enabled.value === true ? 'enabled' : ''));

function onPickStat(key: string) {
  if (key === 'enabled') enabled.value = enabled.value === true ? null : true;
}

const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: rows.value.length,
    forbidden: forbidden.value,
  }),
);
</script>
