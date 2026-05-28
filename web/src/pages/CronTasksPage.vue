<template>
  <q-page class="app-standard-page app-registry-page cron-page">
    <AppPageHero
      kicker="Scheduled tasks"
      title="定时任务"
      subtitle="安排定期 Agent 任务，查看最近执行、失败次数和下一次触发时间。"
    >
      <template #actions>
        <q-btn color="orange" text-color="white" rounded unelevated no-caps icon="add" label="新建任务" @click="openCreate" />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input v-model="search" class="app-page-toolbar__search" dense outlined clearable debounce="300" placeholder="搜索定时任务...">
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select v-model="statusFilter" class="app-page-toolbar__field" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" />
      <div class="app-page-toolbar__meta">共 {{ filteredRows.length }} 个任务，{{ activeCount }} 个启用</div>
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="resetFilters" />
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="loadAll" />
      </template>
    </AppPageToolbar>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadAll" />
      </template>
    </q-banner>

    <q-card v-if="!loading && filteredRows.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="80px" color="grey-9" text-color="grey-5" icon="schedule" />
        <div class="text-h6 q-mt-md">暂无定时任务</div>
        <div class="text-body2 text-grey-7 q-mt-sm">创建定时任务以安排定期 Agent 任务。</div>
        <q-btn class="q-mt-md" color="orange" text-color="white" rounded unelevated no-caps icon="add" label="新建任务" @click="openCreate" />
      </q-card-section>
    </q-card>

    <template v-else>
      <AppRegistryTable
        :rows="pagedRows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        hide-pagination
        :pagination="{ rowsPerPage: 0 }"
      >
        <template #body-cell-name="props">
          <q-td :props="props">
            <div class="app-registry-cell-primary">{{ props.row.name }}</div>
            <div class="app-registry-cell-sub">{{ props.row.key }}</div>
          </q-td>
        </template>

        <template #body-cell-schedule="props">
          <q-td :props="props">
            <span class="app-registry-cell-sub ellipsis" :title="scheduleLabel(props.row)">{{ scheduleLabel(props.row) }}</span>
          </q-td>
        </template>

        <template #body-cell-agent="props">
          <q-td :props="props">
            <span class="app-registry-cell-primary ellipsis">{{ targetLabel(props.row) }}</span>
          </q-td>
        </template>

        <template #body-cell-counts="props">
          <q-td :props="props">
            <div class="app-registry-chip-wrap">
              <q-badge color="grey-7">{{ metadata(props.row).run_count || 0 }} 次</q-badge>
              <q-badge color="positive">{{ metadata(props.row).success_count || 0 }} 成功</q-badge>
              <q-badge :color="(metadata(props.row).failure_count || 0) > 0 ? 'negative' : 'grey-7'">
                {{ metadata(props.row).failure_count || 0 }} 失败
              </q-badge>
            </div>
          </q-td>
        </template>

        <template #body-cell-status="props">
          <q-td :props="props">
            <q-chip dense square :color="statusColor(props.row)" text-color="white">
              {{ props.row.enabled ? props.row.status || "active" : "paused" }}
            </q-chip>
          </q-td>
        </template>

        <template #body-cell-timing="props">
          <q-td :props="props">
            <div class="app-registry-cell-sub">上次 {{ formatDate(metadata(props.row).last_run_at) }}</div>
            <div class="app-registry-cell-sub">下次 {{ formatDate(metadata(props.row).next_run_at) }}</div>
          </q-td>
        </template>

        <template #body-cell-enabled="props">
          <q-td :props="props">
            <q-toggle
              :model-value="props.row.enabled"
              color="primary"
              dense
              :disable="savingId === props.row.id"
              @update:model-value="toggleRow(props.row, Boolean($event))"
            />
          </q-td>
        </template>

        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="app-registry-cell-actions">
              <q-btn flat dense round icon="edit" color="primary" @click="openEdit(props.row)">
                <q-tooltip>编辑</q-tooltip>
              </q-btn>
              <q-btn flat dense round icon="more_vert" color="primary">
                <q-tooltip>更多操作</q-tooltip>
                <q-menu anchor="bottom right" self="top right">
                  <q-list dense style="min-width: 160px">
                    <q-item v-close-popup clickable @click="openRuns(props.row)">
                      <q-item-section avatar><q-icon name="history" /></q-item-section>
                      <q-item-section>执行历史</q-item-section>
                    </q-item>
                    <q-item v-close-popup clickable :disable="triggeringId === props.row.id" @click="runNow(props.row)">
                      <q-item-section avatar><q-icon name="play_arrow" /></q-item-section>
                      <q-item-section>立即执行</q-item-section>
                      <q-item-section v-if="triggeringId === props.row.id" side><q-spinner size="18px" /></q-item-section>
                    </q-item>
                    <q-item v-if="props.row.status === 'dead'" v-close-popup clickable @click="resetDeadTask(props.row)">
                      <q-item-section avatar><q-icon name="restart_alt" color="warning" /></q-item-section>
                      <q-item-section>重置失败计数</q-item-section>
                    </q-item>
                    <q-separator />
                    <q-item v-close-popup clickable class="text-negative" @click="confirmDelete(props.row)">
                      <q-item-section avatar><q-icon name="delete" color="negative" /></q-item-section>
                      <q-item-section>删除</q-item-section>
                    </q-item>
                  </q-list>
                </q-menu>
              </q-btn>
            </div>
          </q-td>
        </template>
      </AppRegistryTable>

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="filteredRows.length"
        :loading="loading"
        label="个任务"
        :page-size-options="[12, 24, 48]"
      />
    </template>

    <CronTaskFormDialog
      v-model="editorOpen"
      :row="editingRow"
      :agents="agents"
      :teams="teams"
      :submitting="formSubmitting"
      :server-error="formServerError"
      @submit="onFormSubmit"
    />
  </q-page>
</template>

<script setup lang="ts">
import AppPageHero from "../components/layout/AppPageHero.vue";
import AppPageToolbar from "../components/layout/AppPageToolbar.vue";
import AppRegistryTable from "../components/layout/AppRegistryTable.vue";
import AppRegistryPagination from "../components/layout/AppRegistryPagination.vue";
import CronTaskFormDialog from "../components/cron/CronTaskFormDialog.vue";
import { useCronTasksPage } from "../features/cron/useCronTasksPage";

const {
  agents,
  teams,
  loading,
  error,
  search,
  statusFilter,
  editorOpen,
  editingRow,
  savingId,
  triggeringId,
  formSubmitting,
  formServerError,
  columns,
  statusOptions,
  activeCount,
  filteredRows,
  pagedRows,
  page,
  pageSize,
  pageMax,
  loadAll,
  onFormSubmit,
  openCreate,
  openEdit,
  toggleRow,
  confirmDelete,
  openRuns,
  resetDeadTask,
  runNow,
  scheduleLabel,
  targetLabel,
  statusColor,
  metadata,
  formatDate
} = useCronTasksPage();

function resetFilters() {
  search.value = "";
  statusFilter.value = "";
  page.value = 1;
}
</script>
