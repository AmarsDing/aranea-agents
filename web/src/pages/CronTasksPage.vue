<template>
  <q-page class="app-standard-page cron-page">
    <!-- 页名在顶栏面包屑里；一行工具条：筛选在左，操作在右 -->
    <section class="cron-toolbar" aria-label="定时任务筛选">
      <q-input
        v-model="search"
        class="cron-toolbar__search"
        dense
        outlined
        clearable
        debounce="200"
        placeholder="搜索名称、消息、计划…"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <div class="cron-seg" role="radiogroup" aria-label="按状态筛选">
        <button
          v-for="opt in stateOptions"
          :key="opt.value"
          type="button"
          role="radio"
          class="cron-seg__item"
          :class="{ 'is-active': stateFilter === opt.value, 'is-dead': opt.value === 'dead' && stateCounts.dead > 0 }"
          :aria-checked="stateFilter === opt.value"
          @click="stateFilter = opt.value"
        >
          {{ opt.label }}<span class="mono">{{ opt.count }}</span>
        </button>
      </div>
      <q-select
        v-if="hasFleet"
        v-model="kindFilter"
        class="cron-toolbar__kind"
        dense
        outlined
        emit-value
        map-options
        :options="kindOptions"
        aria-label="按来源筛选"
      />
      <q-space />
      <q-btn flat dense round icon="refresh" aria-label="刷新" :loading="loading" @click="loadAll">
        <q-tooltip>刷新</q-tooltip>
      </q-btn>
      <q-btn flat dense no-caps icon="history" label="执行记录" @click="openRuns()" />
      <q-btn unelevated dense no-caps color="primary" icon="add" label="新建定时任务" @click="openCreate" />
    </section>

    <AppStatePanel
      v-if="pageState === 'error'"
      variant="error"
      :description="error"
      :action-label="t('common.retry')"
      action-icon="refresh"
      @action="loadAll"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" variant="forbidden" />
    <AppStatePanel v-else-if="pageState === 'loading'" variant="loading" :rows="4" />
    <AppStatePanel
      v-else-if="items.length === 0"
      variant="empty"
      title="还没有定时任务"
      description="定时任务到点把一句话发给 Agent 或 Team，比如每个工作日早上汇总昨天的订单。"
      icon="schedule"
      action-label="新建定时任务"
      action-icon="add"
      @action="openCreate"
    />

    <div v-else class="cron-body" :class="{ 'has-detail': !!selected }">
      <div class="cron-list" role="list" aria-label="定时任务">
        <div class="cron-row cron-row--head" aria-hidden="true">
          <span>任务</span><span>计划</span><span class="cron-col-target">交给</span><span>下次</span><span>上次</span
          ><span>启用</span>
        </div>
        <div
          v-for="it in visibleItems"
          :key="it.key"
          role="listitem"
          tabindex="0"
          class="cron-row"
          :class="[`is-${it.state}`, { 'is-selected': selectedKey === it.key }]"
          :aria-current="selectedKey === it.key ? 'true' : undefined"
          @click="select(it.key)"
          @keydown.enter.prevent="select(it.key)"
        >
          <span class="cron-row__name">
            <i class="cron-dot" :class="`is-${it.state}`" :title="STATE_LABEL[it.state]" />
            <span class="cron-row__text">
              <b>{{ it.name }}</b>
              <small>
                <span v-if="it.kind === 'fleet'" class="cron-tag">运行台</span>
                <span class="mono">{{ it.sub }}</span>
              </small>
            </span>
          </span>
          <span class="cron-row__cell">{{ it.schedule }}</span>
          <span class="cron-row__cell cron-col-target">{{ it.target }}</span>
          <span class="cron-row__cell mono">{{
            it.nextRunAt ? formatCronShort(it.nextRunAt) : it.state === 'active' ? '计算中' : '—'
          }}</span>
          <span class="cron-row__cell">
            <span class="mono">{{ formatCronShort(it.lastRunAt) }}</span>
            <span v-if="it.lastOutcome === 'failure'" class="cron-fail" :title="it.lastError"
              >失败{{ it.failureCount > 1 ? ` ×${it.failureCount}` : '' }}</span
            >
            <span v-else-if="it.lastOutcome === 'skipped'" class="cron-skip">跳过</span>
          </span>
          <span class="cron-row__cell" @click.stop @keydown.stop>
            <q-toggle
              :model-value="it.enabled"
              dense
              :disable="busyKey === it.key || it.state === 'dead'"
              :aria-label="`${it.enabled ? '暂停' : '启用'} ${it.name}`"
              @update:model-value="(v: boolean) => onToggle(it, v)"
            />
          </span>
        </div>
        <p v-if="visibleItems.length === 0" class="cron-list__none">没有符合筛选条件的任务。</p>
      </div>

      <CronTaskDetail
        v-if="selected"
        :item="selected"
        :row="selectedRow"
        :automation="selectedAutomation"
        :runs="recentRuns"
        :runs-loading="runsLoadingRecent"
        :busy="busyKey === selected.key"
        @close="selectedKey = ''"
        @run="onRun"
        @edit="selectedRow && openEdit(selectedRow)"
        @toggle="(v: boolean) => onToggle(selected!, v)"
        @reset="selectedRow && resetDeadTask(selectedRow)"
        @delete="selectedRow && confirmDelete(selectedRow)"
        @history="openRuns(selectedRow ?? undefined)"
        @reload-runs="loadAll"
        @open-fleet="openInFleet"
      />
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
import AppStatePanel from '../components/common/AppStatePanel.vue';
import CronTaskFormDialog from '../components/cron/CronTaskFormDialog.vue';
import CronRunsDialog from '../components/cron/CronRunsDialog.vue';
import CronTaskDetail from '../components/cron/CronTaskDetail.vue';
import { useCronTasksPage } from '../features/cron/useCronTasksPage';
import { STATE_LABEL, type ScheduleItem, type ScheduleState } from '../features/cron/scheduleItems';
import { resolvePageState } from './pageState';

const { t } = useI18n();

const {
  rows,
  agents,
  teams,
  loading,
  error,
  forbidden,
  search,
  stateFilter,
  kindFilter,
  hasFleet,
  items,
  stateCounts,
  visibleItems,
  selectedKey,
  selected,
  selectedRow,
  selectedAutomation,
  recentRuns,
  runsLoadingRecent,
  select,
  editorOpen,
  editingRow,
  busyKey,
  formSubmitting,
  formServerError,
  loadAll,
  onFormSubmit,
  openCreate,
  openEdit,
  toggleTask,
  confirmDelete,
  resetDeadTask,
  runNow,
  toggleAutomationItem,
  runAutomationItem,
  openInFleet,
  formatCronShort,
  runsOpen,
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
  openRuns,
  loadRuns,
  onRunsTaskChange,
  onRunsStatusChange,
  resetRunsFilters,
} = useCronTasksPage();

/** 五态：错误优先于空态；有数据时失败降级为局部提示（列表照常显示）。 */
const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: items.value.length,
    forbidden: forbidden.value,
  }),
);

const stateOptions = computed<{ value: '' | ScheduleState; label: string; count: number }[]>(() => [
  { value: '', label: '全部', count: stateCounts.value.active + stateCounts.value.paused + stateCounts.value.dead },
  { value: 'active', label: '运行中', count: stateCounts.value.active },
  { value: 'paused', label: '已暂停', count: stateCounts.value.paused },
  { value: 'dead', label: '失败停用', count: stateCounts.value.dead },
]);
const kindOptions = [
  { label: '全部来源', value: '' },
  { label: '定时任务（Agent / Team）', value: 'cron' },
  { label: '运行台自动化（编码 agent）', value: 'fleet' },
];

function rowOf(item: ScheduleItem) {
  return rows.value.find((r) => r.id === item.id) ?? null;
}

function onToggle(item: ScheduleItem, enabled: boolean) {
  if (item.kind === 'fleet') return void toggleAutomationItem(item.id, enabled);
  const row = rowOf(item);
  if (row) void toggleTask(row, enabled);
}

function onRun() {
  const item = selected.value;
  if (!item) return;
  if (item.kind === 'fleet') return void runAutomationItem(item.id);
  const row = rowOf(item);
  if (row) void runNow(row);
}
</script>

<style scoped lang="sass">
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
.cron-page
  display: flex
  flex-direction: column
  gap: var(--nt-space-3)
.cron-toolbar
  display: flex
  flex-wrap: wrap
  align-items: center
  gap: var(--nt-space-2)
.cron-toolbar__search
  flex: 0 1 280px
  min-width: 180px
.cron-toolbar__kind
  min-width: 200px
.cron-seg
  display: inline-flex
  padding: 2px
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
.cron-seg__item
  display: inline-flex
  align-items: center
  gap: var(--nt-space-1)
  min-height: 30px
  padding: 0 var(--nt-space-3)
  border: 0
  border-radius: var(--nt-radius-s)
  background: transparent
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
  cursor: pointer
  .mono
    color: var(--nt-text-3)
  &.is-active
    background: var(--nt-accent-soft)
    color: var(--nt-accent)
    .mono
      color: inherit
  &.is-dead .mono
    color: var(--nt-danger)
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.cron-body
  display: grid
  grid-template-columns: minmax(0, 1fr)
  gap: var(--nt-space-3)
  align-items: start
  &.has-detail
    grid-template-columns: minmax(0, 1fr) minmax(360px, 420px)
.cron-list
  min-width: 0
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-l)
  background: var(--nt-glass-panel, var(--nt-bg-1))
  overflow: hidden
.cron-row
  display: grid
  grid-template-columns: minmax(180px, 2fr) minmax(110px, 1.1fr) minmax(130px, 1.3fr) 96px 132px 48px
  align-items: center
  gap: var(--nt-space-3)
  min-height: 52px
  padding: var(--nt-space-2) var(--nt-space-4)
  border-top: 1px solid var(--nt-border-1)
  color: var(--nt-text-1)
  font-size: var(--nt-fs-caption)
  cursor: pointer
  &:hover
    background: var(--nt-bg-2)
  &.is-selected
    background: var(--nt-accent-soft)
    box-shadow: inset 3px 0 0 var(--nt-accent)
  &.is-paused
    color: var(--nt-text-2)
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: -2px
.cron-row--head
  min-height: 36px
  border-top: 0
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
  cursor: default
  &:hover
    background: transparent
.cron-row__name
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  min-width: 0
.cron-row__text
  display: flex
  flex-direction: column
  min-width: 0
  b
    overflow: hidden
    color: var(--nt-text-0)
    font-size: var(--nt-fs-body)
    font-weight: 600
    text-overflow: ellipsis
    white-space: nowrap
  small
    display: flex
    align-items: center
    gap: var(--nt-space-1)
    overflow: hidden
    color: var(--nt-text-3)
    font-size: var(--nt-fs-micro)
    white-space: nowrap
.cron-row__cell
  display: flex
  align-items: baseline
  gap: var(--nt-space-1)
  min-width: 0
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
.cron-dot
  flex: none
  width: 8px
  height: 8px
  border-radius: 50%
  background: var(--nt-text-3)
  &.is-active
    background: var(--nt-success)
  &.is-dead
    background: var(--nt-danger)
.cron-tag
  padding: 0 var(--nt-space-1)
  border: 1px solid var(--nt-border-2)
  border-radius: var(--nt-radius-s)
  color: var(--nt-text-2)
.cron-fail
  color: var(--nt-danger)
.cron-skip
  color: var(--nt-warn)
.cron-list__none
  margin: 0
  padding: var(--nt-space-5)
  color: var(--nt-text-3)
  font-size: var(--nt-fs-caption)
  text-align: center
@media (max-width: 1599px)
  .cron-body.has-detail .cron-col-target
    display: none
  .cron-body.has-detail .cron-row
    grid-template-columns: minmax(160px, 2fr) minmax(100px, 1.2fr) 96px 120px 48px
@media (max-width: 1099px)
  .cron-body.has-detail
    grid-template-columns: minmax(0, 1fr)
  .cron-col-target
    display: none
  .cron-row
    grid-template-columns: minmax(160px, 2fr) minmax(100px, 1.2fr) 96px 120px 48px
@media (max-width: 799px)
  .cron-row--head
    display: none
  .cron-row
    grid-template-columns: 1fr auto
    grid-template-areas: "name toggle" "plan plan"
  .cron-row > :nth-child(1)
    grid-area: name
  .cron-row > :nth-child(2)
    grid-area: plan
  .cron-row > :nth-child(3),
  .cron-row > :nth-child(4),
  .cron-row > :nth-child(5)
    display: none
  .cron-row > :nth-child(6)
    grid-area: toggle
</style>
