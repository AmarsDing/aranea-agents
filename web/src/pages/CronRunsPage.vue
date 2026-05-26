<template>
  <q-page class="app-standard-page app-registry-page cron-runs-page">
    <AppPageHero kicker="Scheduled task runs" title="执行历史" subtitle="查看定时任务触发记录、结果和失败摘要。" />

    <AppPageToolbar>
      <q-select
        v-model="taskId"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="定时任务"
        :options="taskOptions"
        @update:model-value="syncQueryAndLoad"
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
        @update:model-value="syncQueryAndLoad"
      />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" label="清空" @click="resetFilters" />
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="loadRuns" />
      </template>
    </AppPageToolbar>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRuns" />
      </template>
    </q-banner>

    <q-card v-if="!loading && runs.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="history" />
        <div class="text-h6 q-mt-md">暂无执行记录</div>
      </q-card-section>
    </q-card>

    <template v-else>
      <AppRegistryTable
        :rows="pagedRuns"
        :columns="columns"
        row-key="id"
        :loading="loading"
        hide-pagination
        :pagination="{ rowsPerPage: 0 }"
      >
        <template #body-cell-task="props">
          <q-td :props="props">
            <AppRegistryHoverTip :text="props.row.error_message">
              <div class="min-width-0">
                <div class="app-registry-cell-primary">{{ props.row.task_name || taskLabel(props.row.task_id) }}</div>
                <div class="app-registry-cell-sub">{{ props.row.task_id }}</div>
              </div>
            </AppRegistryHoverTip>
          </q-td>
        </template>
        <template #body-cell-time="props">
          <q-td :props="props">
            <div>开始 {{ formatDate(props.row.started_at || props.row.created_at) }}</div>
            <div class="text-caption text-grey-7">结束 {{ formatDate(props.row.finished_at) }}</div>
          </q-td>
        </template>
        <template #body-cell-status="props">
          <q-td :props="props">
            <q-badge :color="runStatusColor(props.row.status)">{{ props.row.status }}</q-badge>
          </q-td>
        </template>
        <template #body-cell-run="props">
          <q-td :props="props">
            <span v-if="props.row.run_id" class="app-registry-cell-sub">{{ props.row.run_id }}</span>
            <span v-else>—</span>
          </q-td>
        </template>
      </AppRegistryTable>

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="runs.length"
        :loading="loading"
        label="条记录"
        :page-size-options="[15, 30, 50]"
      />
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import AppPageHero from "../components/layout/AppPageHero.vue";
import AppPageToolbar from "../components/layout/AppPageToolbar.vue";
import AppRegistryTable from "../components/layout/AppRegistryTable.vue";
import AppRegistryHoverTip from "../components/layout/AppRegistryHoverTip.vue";
import AppRegistryPagination from "../components/layout/AppRegistryPagination.vue";
import { useCronRunsPage } from "../features/cron/useCronRunsPage";

const {
  loading,
  error,
  taskId,
  status,
  taskOptions,
  statusOptions,
  columns,
  runs,
  loadRuns,
  resetFilters,
  syncQueryAndLoad,
  taskLabel,
  runStatusColor,
  formatDate
} = useCronRunsPage();

const page = ref(1);
const pageSize = ref(15);
const pageMax = computed(() => Math.max(1, Math.ceil(runs.value.length / pageSize.value)));
const pagedRuns = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return runs.value.slice(start, start + pageSize.value);
});

watch(runs, () => {
  page.value = 1;
});
</script>
