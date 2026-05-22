<template>
  <q-page class="app-page-cream app-registry-page plugin-runs-page">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">Callback observability</div>
        <h1 class="app-page-title">Callback / Plugin 运行记录</h1>
        <p class="app-page-subtitle">按生命周期点（phase）、Agent、Plugin 与结果筛选；Hook 阻断/错误以 <code>hook:&lt;key&gt;</code> 落库。</p>
      </div>
      <div class="app-actions-bar">
        <q-btn outline rounded no-caps color="primary" icon="rule" label="Hook 规则" to="/hooks" />
        <q-btn outline rounded no-caps color="primary" icon="arrow_back" label="Plugin 管理" to="/plugins" />
      </div>
    </section>

    <q-card flat class="app-registry-panel">
      <q-card-section class="app-form-field-grid app-registry-toolbar items-end">
        <q-select
          v-model="pluginKey"
          class="app-field-md"
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
          class="app-field-md"
          dense
          outlined
          clearable
          debounce="350"
          label="Agent ID"
          @update:model-value="onFilterChange"
        />
        <q-select
          v-model="callbackPoint"
          class="app-field-sm"
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
          class="app-field-sm"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="结果"
          :options="statusOptions"
          @update:model-value="onFilterChange"
        />
        <q-input v-model="from" class="app-field-md" dense outlined clearable type="datetime-local" label="起始时间" @update:model-value="onFilterChange" />
        <q-input v-model="to" class="app-field-md" dense outlined clearable type="datetime-local" label="结束时间" @update:model-value="onFilterChange" />
        <div class="app-actions-bar app-actions-bar--start">
          <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="resetFilters" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="() => loadRows()" />
      </template>
    </q-banner>

    <div class="app-registry-table-shell">
      <q-table
        flat
        dense
        class="app-registry-table"
        :rows="rows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        v-model:pagination="tablePagination"
        :rows-per-page-options="[10, 20, 50]"
        @request="onTableRequest"
      >
        <template #body-cell-plugin_key="props">
          <q-td :props="props">
            <div class="app-registry-cell-primary">{{ props.row.plugin_key }}</div>
          </q-td>
        </template>
        <template #body-cell-status="props">
          <q-td :props="props">
            <q-chip dense :color="statusColor(props.row.status)" text-color="white">{{ props.row.status }}</q-chip>
          </q-td>
        </template>
        <template #body-cell-detail_json="props">
          <q-td :props="props">
            <div class="app-registry-cell-actions">
              <q-btn flat dense size="sm" no-caps label="详情" @click="openDetail(props.row)" />
            </div>
          </q-td>
        </template>
      </q-table>
    </div>

    <q-dialog v-model="detailOpen">
      <q-card class="app-dialog-card app-dialog-card--sm">
        <q-card-section class="text-h6">运行详情</q-card-section>
        <q-card-section class="app-dialog-body q-pt-none">
          <pre class="plugin-run-detail app-code-block">{{ detailText }}</pre>
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat no-caps label="关闭" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { usePluginRunsPage } from "../features/plugins/usePluginRunsPage";

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
  tablePagination,
  detailOpen,
  detailText,
  callbackPointOptions,
  statusOptions,
  pluginKeyOptions,
  columns,
  filterPluginKeys,
  statusColor,
  loadRows,
  onTableRequest,
  onFilterChange,
  resetFilters,
  openDetail
} = usePluginRunsPage();
</script>

<style scoped lang="sass">
.plugin-run-detail
  margin: 0
  white-space: pre-wrap
  overflow-wrap: anywhere
  font-size: var(--text-xs)
</style>
