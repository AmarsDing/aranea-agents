<template>
  <q-page class="app-standard-page app-registry-page plugin-runs-page">
    <AppPageHero
      kicker="Callback observability"
      title="Callback / Plugin 运行记录"
      :subtitle="'按生命周期点（phase）、Agent、Plugin 与结果筛选；Hook 阻断/错误以 hook:<key> 落库。'"
    >
      <template #actions>
        <q-btn outline rounded no-caps color="primary" icon="rule" label="Hook 规则" to="/hooks" />
        <q-btn outline rounded no-caps color="primary" icon="arrow_back" label="Plugin 管理" to="/plugins" />
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
        label="Plugin Key"
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
        label="Agent ID"
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
        label="生命周期点"
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
        label="结果"
        :options="statusOptions"
        @update:model-value="onFilterChange"
      />
      <q-input
        v-model="from"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        type="datetime-local"
        label="起始时间"
        @update:model-value="onFilterChange"
      />
      <q-input
        v-model="to"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        type="datetime-local"
        label="结束时间"
        @update:model-value="onFilterChange"
      />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="resetFilters" />
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="() => loadRows()" />
      </template>
    </AppPageToolbar>

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense label="重试" class="text-white" @click="() => loadRows()" />
      </template>
    </q-banner>

    <AppRegistryTable
      :rows="rows"
      :columns="columns"
      row-key="id"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
    >
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
              aria-label="查看详情"
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
          <q-chip dense :color="statusColor(props.row.status)" text-color="white">{{ props.row.status }}</q-chip>
        </q-td>
      </template>
    </AppRegistryTable>

    <AppRegistryPagination
      v-model:page="page"
      v-model:page-size="pageSize"
      :page-max="pageMax"
      :total="total"
      :loading="loading"
      label="条运行记录"
    />

    <q-dialog v-model="detailOpen">
      <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
        <q-card-section class="text-h6">运行详情</q-card-section>
        <q-card-section class="app-dialog-body q-pt-none">
          <pre class="plugin-run-detail app-code-block">{{ detailText }}</pre>
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn v-close-popup flat no-caps label="关闭" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../components/layout/AppRegistryHoverTip.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import { usePluginRunsPage } from '../features/plugins/usePluginRunsPage';

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
  total,
  page,
  pageSize,
  pageMax,
  detailOpen,
  detailText,
  callbackPointOptions,
  statusOptions,
  pluginKeyOptions,
  columns,
  filterPluginKeys,
  statusColor,
  loadRows,
  onFilterChange,
  resetFilters,
  openDetail,
  detailPreview,
} = usePluginRunsPage();
</script>
