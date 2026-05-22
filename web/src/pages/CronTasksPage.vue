<template>
  <q-page class="app-page-cream app-registry-page cron-page">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">Scheduled tasks</div>
        <h1 class="app-page-title">定时任务</h1>
        <p class="app-page-subtitle">安排定期 Agent 任务，查看最近执行、失败次数和下一次触发时间。</p>
      </div>
      <div class="app-actions-bar">
        <q-btn color="orange" text-color="white" rounded unelevated no-caps icon="add" label="新建任务" @click="openCreate" />
        <q-btn outline rounded no-caps color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadAll" />
      </div>
    </section>

    <q-card flat class="app-registry-panel">
      <q-card-section class="app-form-field-grid app-registry-toolbar items-end">
        <q-input v-model="search" class="app-field-md" dense outlined rounded clearable debounce="300" placeholder="搜索定时任务...">
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <q-select v-model="statusFilter" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" />
        <div class="text-caption text-grey-7">共 {{ filteredRows.length }} 个任务，{{ activeCount }} 个启用</div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadAll" />
      </template>
    </q-banner>

    <div class="app-registry-table-shell">
      <q-table
        v-if="filteredRows.length || loading"
        flat
        dense
        class="app-registry-table"
        row-key="id"
        :rows="filteredRows"
        :columns="columns"
        :loading="loading"
        :pagination="{ rowsPerPage: 12 }"
      >
        <template #body-cell-name="props">
          <q-td :props="props">
            <div class="app-registry-cell-primary">{{ props.row.name }}</div>
            <div class="app-registry-cell-sub">{{ props.row.key }}</div>
          </q-td>
        </template>

        <template #body-cell-description="props">
          <q-td :props="props">
            <div class="app-registry-cell-desc" :title="props.row.description || ''">{{ props.row.description || "—" }}</div>
            <q-tooltip v-if="props.row.description">{{ props.row.description }}</q-tooltip>
          </q-td>
        </template>

        <template #body-cell-schedule="props">
          <q-td :props="props">{{ scheduleLabel(props.row) }}</q-td>
        </template>

        <template #body-cell-agent="props">
          <q-td :props="props">{{ targetLabel(props.row) }}</q-td>
        </template>

        <template #body-cell-counts="props">
          <q-td :props="props">
            <div class="row q-gutter-xs items-center no-wrap">
              <q-badge color="grey-7">{{ metadata(props.row).run_count || 0 }} 次</q-badge>
              <q-badge color="positive">{{ metadata(props.row).success_count || 0 }} 成功</q-badge>
              <q-btn
                flat
                dense
                no-caps
                padding="2px 6px"
                :color="(metadata(props.row).failure_count || 0) > 0 ? 'negative' : 'grey-7'"
                :label="`${metadata(props.row).failure_count || 0} 失败`"
                @click="openRuns(props.row, 'failure')"
              >
                <q-tooltip max-width="320px">
                  <div v-if="recentFailures(props.row).length" class="q-gutter-xs">
                    <div v-for="(failure, index) in recentFailures(props.row)" :key="index">
                      {{ formatDate(failure.started_at) }} · {{ failure.error_message || "未知错误" }}
                    </div>
                  </div>
                  <span v-else>暂无失败记录</span>
                </q-tooltip>
              </q-btn>
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

        <template #body-cell-last="props">
          <q-td :props="props">{{ formatDate(metadata(props.row).last_run_at) }}</q-td>
        </template>

        <template #body-cell-next="props">
          <q-td :props="props">{{ formatDate(metadata(props.row).next_run_at) }}</q-td>
        </template>

        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="app-registry-cell-actions">
            <q-toggle :model-value="props.row.enabled" color="primary" dense :disable="savingId === props.row.id" @update:model-value="toggleRow(props.row, Boolean($event))" />
            <q-btn flat dense round icon="history" color="primary" @click="openRuns(props.row)">
              <q-tooltip>执行历史</q-tooltip>
            </q-btn>
            <q-btn flat dense round icon="play_arrow" color="primary" :loading="triggeringId === props.row.id" @click="runNow(props.row)">
              <q-tooltip>立即执行</q-tooltip>
            </q-btn>
            <q-btn flat dense round icon="edit" color="primary" @click="openEdit(props.row)">
              <q-tooltip>编辑</q-tooltip>
            </q-btn>
            <q-btn v-if="props.row.status === 'dead'" flat dense round icon="restart_alt" color="warning" @click="resetDeadTask(props.row)">
              <q-tooltip>重置失败计数</q-tooltip>
            </q-btn>
            <q-btn flat dense round icon="delete" color="negative" @click="confirmDelete(props.row)">
              <q-tooltip>删除</q-tooltip>
            </q-btn>
            </div>
          </q-td>
        </template>
      </q-table>

      <div v-else class="app-registry-empty cron-empty">
        <q-avatar size="80px" color="grey-9" text-color="grey-5">
          <q-icon name="schedule" size="46px" />
        </q-avatar>
        <div class="text-h6">暂无定时任务</div>
        <div class="text-body2 text-grey-7">创建定时任务以安排定期 Agent 任务。</div>
        <q-btn color="orange" text-color="white" rounded unelevated no-caps icon="add" label="新建任务" @click="openCreate" />
      </div>
    </div>

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
  recentFailures,
  formatDate
} = useCronTasksPage();
</script>

<style scoped>
.cron-empty {
  place-items: center center;
  color: var(--color-text-tertiary);
  display: grid;
  gap: 10px;
  min-height: 280px;
  padding: var(--space-8);
  text-align: center;
}
</style>
