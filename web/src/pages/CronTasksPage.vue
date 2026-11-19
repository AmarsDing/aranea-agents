<template>
  <q-page class="app-standard-page app-registry-page cron-page">
    <AppPageHero kicker="Scheduled tasks" :title="t('cron.title')" :subtitle="t('cron.subtitle')">
      <template #actions>
        <q-btn flat rounded no-caps icon="history" :label="t('cron.runHistory')" @click="openRuns()" />
        <q-btn
          color="accent"
          text-color="white"
          rounded
          unelevated
          no-caps
          icon="add"
          :label="t('cron.createTask')"
          @click="openCreate"
        />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input
        v-model="search"
        class="app-page-toolbar__search"
        dense
        outlined
        clearable
        debounce="300"
        :placeholder="t('cron.searchPlaceholder')"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select
        v-model="statusFilter"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('cron.status')"
        :options="statusOptions"
      />
      <div class="app-page-toolbar__meta">
        {{ t('cron.taskCount', { total: filteredRows.length, active: activeCount }) }}
      </div>
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" :label="t('cron.reset')" @click="resetFilters" />
        <q-btn flat rounded no-caps icon="refresh" :label="t('cron.refresh')" :loading="loading" @click="loadAll" />
      </template>
    </AppPageToolbar>

    <AppStatePanel
      v-if="pageState === 'error'"
      class="q-mb-md"
      variant="error"
      :description="error"
      :action-label="t('common.retry')"
      action-icon="refresh"
      @action="loadAll"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" class="q-mb-md" variant="forbidden" />
    <AppStatePanel v-else-if="pageState === 'loading'" variant="loading" :rows="4" />
    <AppStatePanel
      v-else-if="pageState === 'empty'"
      variant="empty"
      :title="t('cron.emptyTitle')"
      :description="t('cron.emptyHint')"
      icon="schedule"
      :action-label="t('cron.createTask')"
      action-icon="add"
      @action="openCreate"
    />

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
        class="q-mb-md"
        variant="partial"
        dense
        :description="error"
        :action-label="t('common.retry')"
        action-icon="refresh"
        @action="loadAll"
      />
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
            <span class="app-registry-cell-sub ellipsis" :title="scheduleLabel(props.row)">{{
              scheduleLabel(props.row)
            }}</span>
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
              <q-badge color="grey-7">{{ metadata(props.row).run_count || 0 }} {{ t('cron.runCountUnit') }}</q-badge>
              <q-badge color="positive"
                >{{ metadata(props.row).success_count || 0 }} {{ t('cron.successCount') }}</q-badge
              >
              <q-badge :color="(metadata(props.row).failure_count || 0) > 0 ? 'negative' : 'grey-7'">
                {{ metadata(props.row).failure_count || 0 }} {{ t('cron.failureCount') }}
              </q-badge>
            </div>
          </q-td>
        </template>

        <template #body-cell-status="props">
          <q-td :props="props">
            <q-chip dense square :color="statusColor(props.row)" text-color="white">
              {{ props.row.enabled ? props.row.status || 'active' : 'paused' }}
            </q-chip>
          </q-td>
        </template>

        <template #body-cell-timing="props">
          <q-td :props="props">
            <div class="app-registry-cell-sub">
              {{ t('cron.lastRun') }} {{ formatCronDate(metadata(props.row).last_run_at) }}
            </div>
            <div class="app-registry-cell-sub">
              {{ t('cron.nextRun') }} {{ formatCronDate(metadata(props.row).next_run_at) }}
            </div>
          </q-td>
        </template>

        <template #body-cell-enabled="props">
          <q-td :props="props">
            <q-toggle
              :model-value="props.row.enabled"
              color="primary"
              dense
              :disable="savingId !== '' && savingId === props.row.id"
              @update:model-value="toggleRow(props.row, Boolean($event))"
            />
          </q-td>
        </template>

        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="app-registry-cell-actions">
              <q-btn flat dense round icon="edit" color="primary" @click="openEdit(props.row)">
                <q-tooltip>{{ t('cron.editTooltip') }}</q-tooltip>
              </q-btn>
              <q-btn flat dense round icon="more_vert" color="primary">
                <q-tooltip>{{ t('cron.moreActions') }}</q-tooltip>
                <q-menu anchor="bottom right" self="top right">
                  <q-list dense class="app-menu-min-160">
                    <q-item v-close-popup clickable @click="openRuns(props.row)">
                      <q-item-section avatar><q-icon name="history" /></q-item-section>
                      <q-item-section>{{ t('cron.runHistory') }}</q-item-section>
                    </q-item>
                    <q-item v-close-popup clickable :disable="triggeringId !== '' && triggeringId === props.row.id" @click="runNow(props.row)">
                      <q-item-section avatar><q-icon name="play_arrow" /></q-item-section>
                      <q-item-section>{{ t('cron.runNow') }}</q-item-section>
                      <q-item-section v-if="triggeringId !== '' && triggeringId === props.row.id" side
                        ><q-spinner size="18px"
                      /></q-item-section>
                    </q-item>
                    <q-item
                      v-if="props.row.status === 'dead'"
                      v-close-popup
                      clickable
                      @click="resetDeadTask(props.row)"
                    >
                      <q-item-section avatar><q-icon name="restart_alt" color="warning" /></q-item-section>
                      <q-item-section>{{ t('cron.resetFailureCount') }}</q-item-section>
                    </q-item>
                    <q-separator />
                    <q-item v-close-popup clickable class="text-negative" @click="confirmDelete(props.row)">
                      <q-item-section avatar><q-icon name="delete" color="negative" /></q-item-section>
                      <q-item-section>{{ t('common.delete') }}</q-item-section>
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
        :label="t('cron.tasksUnit')"
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

    <CronRunsDialog
      v-model="runsOpen"
      :loading="runsLoading"
      :error="runsError"
      :paged-runs="runsPaged"
      :columns="runsColumns"
      :task-id="runsTaskId"
      :task-options="runsTaskOptions"
      :status="runsStatus"
      :status-options="runsStatusOptions"
      :page="runsPage"
      :page-size="runsPageSize"
      :page-max="runsPageMax"
      :total="runsTotal"
      @update:task-id="onRunsTaskChange"
      @update:status="onRunsStatusChange"
      @update:page="runsPage = $event"
      @update:page-size="runsPageSize = $event"
      @load="loadRuns"
      @reset="resetRunsFilters"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import CronTaskFormDialog from '../components/cron/CronTaskFormDialog.vue';
import CronRunsDialog from '../components/cron/CronRunsDialog.vue';
import { useCronTasksPage } from '../features/cron/useCronTasksPage';
import { resolvePageState } from './pageState';

const { t } = useI18n();

const {
  agents,
  teams,
  loading,
  error,
  forbidden,
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
  formatCronDate,
  runsOpen,
  runs,
  runsTotal,
  runsTaskId,
  runsStatus,
  runsLoading,
  runsError,
  runsColumns,
  runsTaskOptions,
  runsStatusOptions,
  runsPage,
  runsPageSize,
  runsPageMax,
  runsPaged,
  loadRuns,
  onRunsTaskChange,
  onRunsStatusChange,
  resetRunsFilters,
} = useCronTasksPage();

/** 五态判定：错误优先于空态；筛选后为空也算 empty，有数据时失败降级为 partial 横幅。 */
const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: filteredRows.value.length,
    forbidden: forbidden.value,
  }),
);

function resetFilters() {
  search.value = '';
  statusFilter.value = '';
  page.value = 1;
}
</script>
