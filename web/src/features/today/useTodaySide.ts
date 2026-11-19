/**
 * 工作台右栏的数据：今日日程、Agent 定时任务、我的任务、随手记录、交办中、最新邮件。
 * 各块独立读取与降级——一块失败或无权限不影响其余几块。
 * 交办中有两个来源：编程任务看板，和指令栏交办时登记的任务（进度取承载
 * 会话的任务态势）。
 */
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { listCodingTasks } from '../coding/api';
import { listCronTasks, parseCronMetadata } from '../cron/api';
import {
  createOfficeTask,
  listEventsRange,
  listMailMessages,
  listOfficeTasks,
  setOfficeTaskStatus,
} from '../office/api';
import type { MailMessageRow, OfficeEventRow, OfficeTaskRow } from '../office/types';
import { listTaskSituations } from '../session/api';
import { useNavStore } from '../../stores/nav';
import { parseKratosApiError } from '../../utils/kratosError';
import {
  classifyDelegated,
  classifyOfficeDelegated,
  dayBounds,
  sortDelegated,
  type CronNextRun,
  type DelegatedRow,
  type SituationInput,
} from './logic';

/** ok = 读取成功；denied = 无权限（不发请求）；error = 读取失败（保留旧数据） */
export type BlockStatus = 'loading' | 'ok' | 'denied' | 'error';

const POLL_INTERVAL_MS = 60_000;
const DELEGATED_FETCH_LIMIT = 100;
const NOTES_SHOWN = 5;
const MAILS_SHOWN = 5;

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
  const notes = ref<OfficeTaskRow[]>([]);
  const mails = ref<MailMessageRow[]>([]);
  const mailTotal = ref(0);
  const mailsStatus = ref<BlockStatus>('loading');

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

  // 态势只是进度补充：读不到时交办条目按「进行中」显示，不让整块报错。
  async function loadSituations(): Promise<SituationInput[]> {
    try {
      return (await listTaskSituations({ limit: DELEGATED_FETCH_LIMIT })).items;
    } catch (err) {
      console.warn('[today] task situations load failed', err);
      return [];
    }
  }

  async function loadDelegated() {
    const [coding, office] = await Promise.allSettled([
      listCodingTasks(DELEGATED_FETCH_LIMIT),
      listOfficeTasks('open', DELEGATED_FETCH_LIMIT, 'delegate'),
    ]);
    if (coding.status === 'rejected') console.warn('[today] coding tasks load failed', coding.reason);
    if (office.status === 'rejected') console.warn('[today] delegated tasks load failed', office.reason);
    if (coding.status === 'rejected' && office.status === 'rejected') {
      delegatedStatus.value = 'error';
      return;
    }
    const rows: DelegatedRow[] = [];
    if (coding.status === 'fulfilled') rows.push(...classifyDelegated(coding.value, new Date()));
    if (office.status === 'fulfilled' && office.value.length > 0) {
      rows.push(...classifyOfficeDelegated(office.value, await loadSituations()));
    }
    delegated.value = sortDelegated(rows);
    delegatedStatus.value = 'ok';
  }

  // 随手记录只显示最近几条，读取失败时静默保留上一次的内容。
  async function loadNotes() {
    try {
      notes.value = await listOfficeTasks('open', NOTES_SHOWN, 'note');
    } catch (err) {
      console.warn('[today] notes load failed', err);
    }
  }

  async function loadMails() {
    if (!can('office.mail')) {
      mails.value = [];
      mailsStatus.value = 'denied';
      return;
    }
    try {
      const res = await listMailMessages({ pageSize: MAILS_SHOWN });
      mails.value = res.items;
      mailTotal.value = res.total;
      mailsStatus.value = 'ok';
    } catch (err) {
      console.warn('[today] mails load failed', err);
      mailsStatus.value = 'error';
    }
  }

  async function refresh(): Promise<void> {
    await Promise.all([loadEvents(), loadCron(), loadTasks(), loadDelegated(), loadNotes(), loadMails()]);
  }

  /** 记一笔；失败时返回错误文案，成功返回空串。 */
  async function addNote(text: string): Promise<string> {
    const trimmed = text.trim();
    if (!trimmed) return '';
    try {
      const created = await createOfficeTask(trimmed, '', 'note');
      notes.value = [created, ...notes.value].slice(0, NOTES_SHOWN);
      return '';
    } catch (err) {
      return parseKratosApiError(err).message || 'error';
    }
  }

  /** 收起一条记录（标记完成后不再显示）。 */
  async function dismissNote(id: string): Promise<void> {
    try {
      await setOfficeTaskStatus(id, 'done');
      notes.value = notes.value.filter((note) => note.id !== id);
      void loadNotes();
    } catch (err) {
      console.warn('[today] note dismiss failed', err);
    }
  }

  /** 指令栏刚交办成功：先把这条放进「交办中」，再向服务端对一次账。 */
  /** 别处（指令栏、Ctrl+K 指令面板）刚建好的任务 / 记录 / 交办，就地放进对应列表。 */
  function ingest(intent: 'task' | 'note' | 'delegate', task: OfficeTaskRow | null): void {
    if (!task) {
      if (intent === 'delegate') void loadDelegated();
      return;
    }
    if (intent === 'delegate') trackDelegation(task);
    else if (intent === 'note')
      notes.value = [task, ...notes.value.filter((n) => n.id !== task.id)].slice(0, NOTES_SHOWN);
    else tasks.value = [task, ...tasks.value.filter((x) => x.id !== task.id)];
  }

  function trackDelegation(task: OfficeTaskRow): void {
    const [row] = classifyOfficeDelegated([task], []);
    if (row) delegated.value = sortDelegated([row, ...delegated.value.filter((d) => d.id !== row.id)]);
    delegatedStatus.value = 'ok';
    void loadDelegated();
  }

  /** 验收通过一条指令栏交办的任务；失败时返回错误文案，成功返回空串。 */
  async function acceptDelegated(officeTaskId: string): Promise<string> {
    if (taskBusy.value.has(officeTaskId)) return '';
    taskBusy.value = new Set(taskBusy.value).add(officeTaskId);
    try {
      await setOfficeTaskStatus(officeTaskId, 'done');
      delegated.value = delegated.value.filter((row) => row.officeTaskId !== officeTaskId);
      return '';
    } catch (err) {
      return parseKratosApiError(err).message || 'error';
    } finally {
      const next = new Set(taskBusy.value);
      next.delete(officeTaskId);
      taskBusy.value = next;
    }
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
    ingest,
    events,
    eventsStatus,
    cron,
    tasks,
    tasksStatus,
    taskBusy,
    delegated,
    delegatedStatus,
    delegatedCounts,
    notes,
    mails,
    mailTotal,
    mailsStatus,
    refresh,
    addTask,
    addNote,
    dismissNote,
    trackDelegation,
    acceptDelegated,
    completeTask,
    reopenTask,
  };
}
