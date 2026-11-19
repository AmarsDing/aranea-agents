<template>
  <q-page class="app-standard-page app-registry-page plugin-runs-page">
    <AppPageHero
      :kicker="t('pluginsPage.runs.kicker')"
      :title="t('pluginsPage.runs.title')"
      :subtitle="t('pluginsPage.runs.subtitle')"
    >
      <template #actions>
        <q-btn
          outline
          rounded
          no-caps
          color="primary"
          icon="rule"
          :label="t('pluginsPage.runs.btnHookRules')"
          to="/hooks"
        />
        <q-btn
          outline
          rounded
          no-caps
          color="primary"
          icon="arrow_back"
          :label="t('pluginsPage.runs.btnPlugins')"
          to="/plugins"
        />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-select
        v-model="pluginKey"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        use-input
        fill-input
        hide-selected
        input-debounce="0"
        :label="t('pluginsPage.runs.filterPluginKey')"
        :options="pluginKeyOptions"
        @filter="filterPluginKeys"
        @update:model-value="onFilterChange"
      />
      <q-input
        v-model="agentId"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        debounce="350"
        :label="t('pluginsPage.runs.filterAgentId')"
        @update:model-value="onFilterChange"
      />
      <q-select
        v-model="callbackPoint"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('pluginsPage.runs.filterCallbackPoint')"
        :options="callbackPointOptions"
        @update:model-value="onFilterChange"
      />
      <q-select
        v-model="status"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('pluginsPage.runs.filterStatus')"
        :options="statusOptions"
        @update:model-value="onFilterChange"
      />
      <q-input
        v-model="from"
        class="app-page-toolbar__field app-page-toolbar__field--time"
        dense
        outlined
        clearable
        type="datetime-local"
        :label="t('pluginsPage.runs.filterFrom')"
        @update:model-value="onFilterChange"
      />
      <q-input
        v-model="to"
        class="app-page-toolbar__field app-page-toolbar__field--time"
        dense
        outlined
        clearable
        type="datetime-local"
        :label="t('pluginsPage.runs.filterTo')"
        @update:model-value="onFilterChange"
      />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" :label="t('pluginsPage.btnReset')" @click="resetFilters" />
        <q-btn
          flat
          rounded
          no-caps
          icon="refresh"
          :label="t('pluginsPage.btnRefresh')"
          :loading="loading"
          @click="() => loadRows()"
        />
        <q-btn
          flat
          rounded
          no-caps
          icon="delete_sweep"
          :label="t('pluginsPage.runs.btnClear')"
          :loading="clearing"
          :disable="rows.length === 0"
          @click="confirmClear"
        />
      </template>
    </AppPageToolbar>

    <AppStatePanel
      v-if="pageState === 'error'"
      class="q-mb-md"
      variant="error"
      :description="error"
      :action-label="t('common.retry')"
      action-icon="refresh"
      @action="() => loadRows()"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" class="q-mb-md" variant="forbidden" />
    <AppStatePanel v-else-if="pageState === 'loading'" variant="loading" :rows="4" />
    <AppStatePanel
      v-else-if="pageState === 'empty'"
      variant="empty"
      :title="t('shell.runs.emptyTitle')"
      :description="t('shell.runs.emptyDesc')"
      icon="history"
      :action-label="t('shell.runs.emptyAction')"
      action-icon="restart_alt"
      @action="resetFilters"
    />

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
        class="q-mb-md"
        variant="partial"
        dense
        :description="t('shell.runs.staleError', { message: error })"
        :action-label="t('common.retry')"
        action-icon="refresh"
        @action="() => loadRows()"
      />

      <AppRegistryTable :rows="rows" :columns="columns" row-key="id" :loading="loading" hide-pagination :pagination="{ rowsPerPage: 0 }">
        <template #body-cell-plugin_key="props">
          <q-td :props="props">
            <div class="row items-center no-wrap q-gutter-xs">
              <AppRegistryHoverTip :text="detailPreview(props.row)" :indicator="Boolean(detailPreview(props.row))">
                <span class="app-registry-cell-primary ellipsis">{{ props.row.plugin_key }}</span>
              </AppRegistryHoverTip>
              <q-btn
                v-if="detailPreview(props.row)"
                flat
                dense
                round
                icon="visibility"
                color="primary"
                :aria-label="t('pluginsPage.runs.viewDetail')"
                @click="openDetail(props.row)"
              />
            </div>
          </q-td>
        </template>
        <template #body-cell-agent_id="props">
          <q-td :props="props">
            <span class="app-registry-cell-sub ellipsis" :title="props.row.agent_id">{{
              props.row.agent_id || '—'
            }}</span>
          </q-td>
        </template>
        <template #body-cell-status="props">
          <q-td :props="props">
            <AppStatusChip :status="props.row.status" />
          </q-td>
        </template>
      </AppRegistryTable>

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="total"
        :loading="loading"
        :label="t('pluginsPage.runs.paginationLabel')"
      />
    </template>

    <PluginRunDetailDialog v-model:open="detailOpen" :text="detailText" />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { Dialog } from 'quasar';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../components/layout/AppRegistryHoverTip.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import PluginRunDetailDialog from '../components/plugins/PluginRunDetailDialog.vue';
import AppStatusChip from '../components/common/AppStatusChip.vue';
import { usePluginRunsPage } from '../features/plugins/usePluginRunsPage';
import { resolvePageState } from './pageState';

const { t } = useI18n();

const {
  pluginKey,
  agentId,
  callbackPoint,
  status,
  from,
  to,
  rows,
  loading,
  error,
  forbidden,
  total,
  page,
  pageSize,
  pageMax,
  detailOpen,
  detailText,
  clearing,
  callbackPointOptions,
  statusOptions,
  pluginKeyOptions,
  columns,
  filterPluginKeys,
  loadRows,
  onFilterChange,
  resetFilters,
  openDetail,
  detailPreview,
  clearAll,
} = usePluginRunsPage();

/** 五态判定：错误优先于空态；有数据时刷新失败只加 partial 横幅，不清空已渲染列表。 */
const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: rows.value.length,
    forbidden: forbidden.value,
  }),
);

function confirmClear() {
  Dialog.create({
    title: t('pluginsPage.runs.confirmClearTitle'),
    message: t('pluginsPage.runs.confirmClearMessage'),
    cancel: { label: t('pluginsPage.runs.cancel'), flat: true, noCaps: true },
    ok: { label: t('pluginsPage.runs.confirmClearOk'), color: 'negative', flat: true, noCaps: true },
    persistent: true,
  }).onOk(() => void clearAll());
}
</script>

<style scoped lang="sass">
// 6 个筛选字段 + 操作按钮：字段禁止收缩（避免标签截断），空间不足时换行
:deep(.app-page-toolbar__body)
  flex-wrap: wrap

:deep(.app-page-toolbar__field)
  flex: 0 0 auto
  min-width: 150px

:deep(.app-page-toolbar__field--time)
  min-width: 200px
</style>
