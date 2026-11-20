import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { confirm } from '../../composables/useAppConfirm';

import { parseCronConfig, parseCronMetadata } from './api';
import type { PlatformResourceInput } from '../platform/types';
import type { CronTaskConfig, CronTaskMetadata, CronTaskRow } from './types';
import { CRON_RUN_TABLE_COLUMNS } from './cronTableUi';
import { useCronStore } from '../../stores/cron';
import { formatCronDate, formatCronShort } from '../../components/cron/cronTaskUtils';
import { isForbiddenApiError } from '../../utils/kratosError';
import { cronItem, fleetItem, sortScheduleItems, type ScheduleItem, type ScheduleState } from './scheduleItems';

export type ScheduleKindFilter = '' | 'cron' | 'fleet';

export function useCronTasksPage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const router = useRouter();
  const cronStore = useCronStore();
  const {
    tasks: rows,
    agents,
    teams,
    runs,
    runsTotal,
    recentRuns,
    automations,
    fleetProjects,
  } = storeToRefs(cronStore);

  const loading = ref(false);
  const error = ref('');
  /** 403 / PERMISSION_DENIED：页面渲染 forbidden 态，不提供重试。 */
  const forbidden = ref(false);
  const search = ref('');
  const stateFilter = ref<'' | ScheduleState>('');
  const kindFilter = ref<ScheduleKindFilter>('');
  const editorOpen = ref(false);
  const editingRow = ref<CronTaskRow | null>(null);
  const busyKey = ref('');
  const formSubmitting = ref(false);
  const formServerError = ref('');

  // ---------- 统一列表 ----------

  const items = computed<ScheduleItem[]>(() => [
    ...rows.value.map((r) => cronItem(r, agents.value, teams.value)),
    ...(automations.value ?? []).map((a) => fleetItem(a, fleetProjects.value)),
  ]);
  const hasFleet = computed(() => automations.value !== null);

  /** 先按来源 + 关键字筛，状态计数基于这一层（点状态筛选不会让其它状态的数变成 0）。 */
  const baseRows = computed(() => {
    const keyword = search.value.trim().toLowerCase();
    return items.value.filter(
      (it) => (!kindFilter.value || it.kind === kindFilter.value) && (!keyword || it.searchText.includes(keyword)),
    );
  });
  const stateCounts = computed(() => {
    const c = { active: 0, paused: 0, dead: 0 } as Record<ScheduleState, number>;
    for (const it of baseRows.value) c[it.state]++;
    return c;
  });
  const visibleItems = computed(() =>
    sortScheduleItems(baseRows.value.filter((it) => !stateFilter.value || it.state === stateFilter.value)),
  );

  // ---------- 选中与详情 ----------

  const selectedKey = ref('');
  const selected = computed(() => items.value.find((it) => it.key === selectedKey.value) ?? null);
  const selectedRow = computed(() =>
    selected.value?.kind === 'cron' ? (rows.value.find((r) => r.id === selected.value?.id) ?? null) : null,
  );
  const selectedAutomation = computed(() =>
    selected.value?.kind === 'fleet'
      ? ((automations.value ?? []).find((a) => a.id === selected.value?.id) ?? null)
      : null,
  );
  const runsLoadingRecent = ref(false);

  function select(key: string) {
    selectedKey.value = selectedKey.value === key ? '' : key;
  }

  async function loadRecent() {
    const row = selectedRow.value;
    if (!row) return;
    runsLoadingRecent.value = true;
    try {
      await cronStore.loadRecentRuns(row.id);
    } catch (err) {
      console.warn('[cron] recent runs load failed', err);
    } finally {
      runsLoadingRecent.value = false;
    }
  }
  watch(
    () => selectedRow.value?.id,
    (id) => {
      recentRuns.value = [];
      if (id) void loadRecent();
    },
  );

  onMounted(() => void loadAll());

  // 上次 / 下次时间由调度器在后台写（新建或改计划后约 1 分钟内算出）：页面可见时每分钟静默刷新一次列表。
  // 编辑中不刷，免得表单底下的数据跳动；失败不提示，下一轮再试。
  const quietTimer = setInterval(() => {
    if (document.hidden || editorOpen.value || loading.value) return;
    void Promise.all([cronStore.loadTasks(), cronStore.loadAutomations()]).catch(() => undefined);
  }, 60_000);
  onBeforeUnmount(() => clearInterval(quietTimer));

  watch(editorOpen, (open) => {
    if (open) formServerError.value = '';
  });

  async function loadAll() {
    loading.value = true;
    error.value = '';
    forbidden.value = false;
    try {
      await Promise.all([cronStore.loadAll(), cronStore.loadAutomations()]);
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载定时任务失败';
      forbidden.value = isForbiddenApiError(err);
    } finally {
      loading.value = false;
    }
    if (selectedRow.value) void loadRecent();
  }

  // ---------- 定时任务：增删改与执行 ----------

  async function onFormSubmit(payload: PlatformResourceInput) {
    formServerError.value = '';
    formSubmitting.value = true;
    try {
      if (editingRow.value) {
        await cronStore.editTask(editingRow.value.id, payload);
      } else {
        const created = await cronStore.addTask(payload);
        selectedKey.value = `cron:${created.id}`;
      }
      editorOpen.value = false;
      $q.notify({ type: 'positive', message: '定时任务已保存' });
    } catch (err) {
      formServerError.value = err instanceof Error ? err.message : '保存失败';
      $q.notify({ type: 'negative', message: formServerError.value });
    } finally {
      formSubmitting.value = false;
    }
  }

  function openCreate() {
    editingRow.value = null;
    editorOpen.value = true;
  }

  function openEdit(row: CronTaskRow) {
    editingRow.value = row;
    editorOpen.value = true;
  }

  async function toggleTask(row: CronTaskRow, enabled: boolean) {
    busyKey.value = `cron:${row.id}`;
    try {
      await cronStore.editTask(row.id, { enabled, status: enabled ? 'active' : 'paused' });
      $q.notify({ type: 'positive', message: enabled ? '任务已启用，下次时间按计划重新计算' : '任务已暂停' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '更新失败' });
    } finally {
      busyKey.value = '';
    }
  }

  async function confirmDelete(row: CronTaskRow) {
    const ok = await confirm({
      title: '删除定时任务',
      message: `确定删除「${row.name}」？此操作不可撤销。`,
      tone: 'danger',
    });
    if (!ok) return;
    try {
      await cronStore.removeTask(row.id);
      if (selectedKey.value === `cron:${row.id}`) selectedKey.value = '';
      $q.notify({ type: 'positive', message: '任务已删除' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '删除失败' });
    }
  }

  async function resetDeadTask(row: CronTaskRow) {
    busyKey.value = `cron:${row.id}`;
    try {
      await cronStore.resetFailures(row.id);
      $q.notify({ type: 'positive', message: '失败计数已清零，任务恢复运行' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '重置失败' });
    } finally {
      busyKey.value = '';
    }
  }

  async function runNow(row: CronTaskRow) {
    busyKey.value = `cron:${row.id}`;
    try {
      const run = await cronStore.triggerTask(row.id);
      $q.notify({
        type: run.status === 'failure' ? 'negative' : 'info',
        message: run.status === 'failure' ? run.error_message || '执行失败' : '已开始执行，结果会出现在右侧最近执行里',
      });
      selectedKey.value = `cron:${row.id}`;
      await loadAll();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '触发失败' });
    } finally {
      busyKey.value = '';
    }
  }

  // ---------- 运行台自动化 ----------

  async function toggleAutomationItem(id: string, enabled: boolean) {
    const a = (automations.value ?? []).find((x) => x.id === id);
    if (!a) return;
    busyKey.value = `fleet:${id}`;
    try {
      await cronStore.toggleAutomation(a, enabled);
      $q.notify({ type: 'positive', message: enabled ? '自动化已启用' : '自动化已暂停' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '更新失败' });
    } finally {
      busyKey.value = '';
    }
  }

  async function runAutomationItem(id: string) {
    busyKey.value = `fleet:${id}`;
    try {
      const a = await cronStore.runAutomation(id);
      const failed = a.lastStatus.startsWith('failed');
      $q.notify({ type: failed ? 'negative' : 'positive', message: failed ? a.lastStatus : '已在运行台启动 agent' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '启动失败' });
    } finally {
      busyKey.value = '';
    }
  }

  function openInFleet(workspaceId = '') {
    void router.push({ name: 'fleet', query: workspaceId ? { ws: workspaceId } : { view: 'automations' } });
  }

  // ---------- 全部执行记录（弹窗，服务端分页） ----------

  const runsOpen = ref(false);
  const runsTaskId = ref('');
  const runsStatus = ref('');
  const runsLoading = ref(false);
  const runsError = ref('');
  const runsPage = ref(1);
  const runsPageSize = ref(15);

  const runsColumns = CRON_RUN_TABLE_COLUMNS;
  const runsTaskOptions = computed(() => rows.value.map((task) => ({ label: task.name, value: task.id })));
  const runsStatusOptions = [
    { label: '成功', value: 'success' },
    { label: '失败', value: 'failure' },
    { label: '跳过', value: 'skipped' },
    // 新键走 i18n（check-i18n 不新增违例）；存量硬编码标签属既有 baseline，不在本改动迁移
    { label: t('common.status.skippedMissed'), value: 'skipped_missed' },
    { label: '执行中', value: 'pending' },
  ];
  const runsPageMax = computed(() => Math.max(1, Math.ceil(runsTotal.value / runsPageSize.value)));
  const runsPaged = runs;

  watch([runsPage, runsPageSize], () => {
    if (runsOpen.value) void loadRuns();
  });

  function openRuns(row?: CronTaskRow, status = '') {
    runsTaskId.value = row?.id || '';
    runsStatus.value = status;
    runsPage.value = 1;
    runsOpen.value = true;
    void loadRuns();
  }

  async function loadRuns() {
    runsLoading.value = true;
    runsError.value = '';
    try {
      await cronStore.loadRuns({
        cron_task_id: runsTaskId.value || undefined,
        status: runsStatus.value || undefined,
        page: runsPage.value,
        page_size: runsPageSize.value,
        // 历史要看全：在工作台告警里「忽略」过的失败也照常列出（带已忽略标记）
        include_resolved: true,
      });
    } catch (err) {
      runsError.value = err instanceof Error ? err.message : '加载执行历史失败';
    } finally {
      runsLoading.value = false;
    }
  }

  function onRunsTaskChange(value: string) {
    runsTaskId.value = value;
    runsPage.value = 1;
    void loadRuns();
  }

  function onRunsStatusChange(value: string) {
    runsStatus.value = value;
    runsPage.value = 1;
    void loadRuns();
  }

  function resetRunsFilters() {
    runsTaskId.value = '';
    runsStatus.value = '';
    runsPage.value = 1;
    void loadRuns();
  }

  function config(row: CronTaskRow): CronTaskConfig {
    return parseCronConfig(row);
  }

  function metadata(row: CronTaskRow): CronTaskMetadata {
    return parseCronMetadata(row);
  }

  return {
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
    config,
    metadata,
    formatCronDate,
    formatCronShort,
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
    openRuns,
    loadRuns,
    onRunsTaskChange,
    onRunsStatusChange,
    resetRunsFilters,
  };
}
