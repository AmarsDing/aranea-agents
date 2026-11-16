/**
 * 工作台右栏的数据：今日日程、Agent 定时任务、我的任务、交办中。
 * 四块各自独立读取与降级——一块失败或无权限不影响其余三块。
 */
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { listCodingTasks } from '../coding/api';
import { listCronTasks, parseCronMetadata } from '../cron/api';
import { createOfficeTask, listEventsRange, listOfficeTasks, setOfficeTaskStatus } from '../office/api';
import type { OfficeEventRow, OfficeTaskRow } from '../office/types';
import { useNavStore } from '../../stores/nav';
import { parseKratosApiError } from '../../utils/kratosError';
import { classifyDelegated, dayBounds, type CronNextRun, type DelegatedRow } from './logic';

/** ok = 读取成功；denied = 无权限（不发请求）；error = 读取失败（保留旧数据） */
export type BlockStatus = 'loading' | 'ok' | 'denied' | 'error';

const POLL_INTERVAL_MS = 60_000;
const DELEGATED_FETCH_LIMIT = 100;

export function useTodaySide() {
  const nav = useNavStore();
  const can = (code: string) => nav.permissions.includes('*') || nav.permissions.includes(code);

  const events = ref<OfficeEventRow[]>([]);
  const eventsStatus = ref<BlockStatus>('loading');
  const cron = ref<CronNextRun[]>([]);
  const tasks = ref<OfficeTaskRow[]>([]);
  const tasksStatus = ref<BlockStatus>('loading');
  const taskBusy = ref<Set<string>>(new Set());
  const delegated = ref<DelegatedRow[]>([]);
  const delegatedStatus = ref<BlockStatus>('loading');

  let timer: ReturnType<typeof setInterval> | null = null;

  async function loadEvents() {
    if (!can('office.schedule')) {
      events.value = [];
      eventsStatus.value = 'denied';
      return;
    }
    try {
      const { start, end } = dayBounds(new Date());
      events.value = await listEventsRange(start.toISOString(), end.toISOString());
      eventsStatus.value = 'ok';
    } catch (err) {
      console.warn('[today] events load failed', err);
      eventsStatus.value = 'error';
    }
  }

  // 定时任务只是时间线上的补充信息：无权限或失败时静默留空，不单独报错。
  async function loadCron() {
    if (!can('system.cron')) {
      cron.value = [];
      return;
    }
    try {
      const rows = await listCronTasks();
      cron.value = rows
        .filter((row) => row.enabled)
        .map((row) => ({ id: row.id, name: row.name || row.key, nextRunAt: parseCronMetadata(row).next_run_at ?? '' }))
        .filter((job) => job.nextRunAt);
    } catch (err) {
      console.warn('[today] cron load failed', err);
    }
  }

  async function loadTasks() {
    try {
      tasks.value = await listOfficeTasks('open', 50);
      tasksStatus.value = 'ok';
    } catch (err) {
      console.warn('[today] tasks load failed', err);
      tasksStatus.value = 'error';
    }
  }

  async function loadDelegated() {
    try {
      delegated.value = classifyDelegated(await listCodingTasks(DELEGATED_FETCH_LIMIT), new Date());
      delegatedStatus.value = 'ok';
    } catch (err) {
      console.warn('[today] delegated load failed', err);
      delegatedStatus.value = 'error';
    }
  }

  async function refresh(): Promise<void> {
    await Promise.all([loadEvents(), loadCron(), loadTasks(), loadDelegated()]);
  }

  /** 新建一条我的任务；失败时返回错误文案，成功返回空串。 */
  async function addTask(title: string): Promise<string> {
    const trimmed = title.trim();
    if (!trimmed) return '';
    try {
      const created = await createOfficeTask(trimmed);
      tasks.value = [created, ...tasks.value];
      return '';
    } catch (err) {
      return parseKratosApiError(err).message || 'error';
    }
  }

  /** 勾选完成：成功后从列表移除；失败时保留并返回错误文案。 */
  async function completeTask(id: string): Promise<string> {
    if (taskBusy.value.has(id)) return '';
    taskBusy.value = new Set(taskBusy.value).add(id);
    try {
      await setOfficeTaskStatus(id, 'done');
      tasks.value = tasks.value.filter((task) => task.id !== id);
      return '';
    } catch (err) {
      return parseKratosApiError(err).message || 'error';
    } finally {
      const next = new Set(taskBusy.value);
      next.delete(id);
      taskBusy.value = next;
    }
  }

  /** 撤销刚才的完成（重开任务）。 */
  async function reopenTask(id: string): Promise<void> {
    try {
      const reopened = await setOfficeTaskStatus(id, 'open');
      if (!tasks.value.some((task) => task.id === id)) tasks.value = [reopened, ...tasks.value];
    } catch (err) {
      console.warn('[today] task reopen failed', err);
    }
  }

  const delegatedCounts = computed(() => {
    const out = { stuck: 0, review: 0, running: 0 };
    for (const row of delegated.value) out[row.state] += 1;
    return out;
  });

  function onVisibility() {
    if (!document.hidden) void refresh();
  }

  onMounted(() => {
    void refresh();
    timer = setInterval(() => {
      if (!document.hidden) void refresh();
    }, POLL_INTERVAL_MS);
    document.addEventListener('visibilitychange', onVisibility);
  });

  watch(
    () => nav.permissions.join(','),
    () => void refresh(),
  );

  onUnmounted(() => {
    if (timer) clearInterval(timer);
    document.removeEventListener('visibilitychange', onVisibility);
  });

  return {
    events,
    eventsStatus,
    cron,
    tasks,
    tasksStatus,
    taskBusy,
    delegated,
    delegatedStatus,
    delegatedCounts,
    refresh,
    addTask,
    completeTask,
    reopenTask,
  };
}
