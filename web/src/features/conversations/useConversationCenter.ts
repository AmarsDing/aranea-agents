// 会话中心的数据：读各来源 → 拼成任务（model.ts）→ 定时刷新；以及按来源落到各自接口的归档、删除、恢复。
// 只用现成接口（P1）。读不到的来源不拦整页，只在 errors 里记一笔。
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import { listAgents } from '../agents/api';
import { cancelCodingTask, listCodingTasks, reviewCodingTask } from '../coding/api';
import { closeTerminal, getRun, listRuns, type RunDetail } from '../fleet/api';
import { useFleet } from '../fleet/useFleet';
import { listOfficeTasks, setOfficeTaskStatus } from '../office/api';
import {
  archiveSession,
  deleteSession,
  getSession,
  pinSession,
  restoreSession,
  searchSessions,
  unpinSession,
  updateSessionMetadata,
} from '../session/api';
import type { Session } from '../session/types';
import { listTeams } from '../teams/api';
import { COMMAND_DONE_EVENT } from '../today/useCommandRunner';
import { useNavStore } from '../../stores/nav';
import type { CodingTask } from '../../services/kratos/agentbridge/v1';
import type { OfficeTaskRow } from '../office/types';
import { mergeMeta } from './handoff';
import { buildTasks, flat, type Bucket, type ConvTask, type Execution } from './model';

const POLL_MS = 30_000;
const SESSION_LIMIT = 200;
const CODING_LIMIT = 100;
/** 只展开最近的几次编排（每次要单独读一次详情） */
const RUN_LIMIT = 10;

export type SourceKey = 'sessions' | 'office' | 'coding' | 'fleet';

export interface ActionResult {
  done: number;
  /** 这个来源不支持该操作、或当前状态不允许（如还在跑的会话不能归档） */
  skipped: number;
  failed: number;
}

export function useConversationCenter() {
  const nav = useNavStore();
  const fleet = useFleet();
  const canFleet = computed(() => nav.permissions.includes('*') || nav.permissions.includes('coding.manage'));

  const sessions = reactive(new Map<Bucket, Session[]>());
  const agentNames = ref<Record<string, string>>({});
  const teamNames = ref<Record<string, string>>({});
  const officeTasks = ref<OfficeTaskRow[]>([]);
  const codingTasks = ref<CodingTask[]>([]);
  const runs = ref<RunDetail[]>([]);
  const loaded = ref(false);
  const errors = ref<SourceKey[]>([]);
  /** 已归档 / 回收站的会话按需读（切到那两个视图或搜索时） */
  const buckets = ref<Bucket[]>(['active']);

  const tasks = computed<ConvTask[]>(() =>
    buildTasks({
      sessions: buckets.value.flatMap((b) => sessions.get(b) ?? []),
      agentNames: agentNames.value,
      teamNames: teamNames.value,
      officeTasks: officeTasks.value,
      codingTasks: codingTasks.value,
      runs: runs.value,
      terminals: canFleet.value ? fleet.state.terminals : [],
      presets: fleet.state.presets,
      workspaces: fleet.state.workspaces,
    }),
  );

  async function loadNames() {
    const [agents, teams] = await Promise.allSettled([listAgents({ limit: 500 }), listTeams()]);
    if (agents.status === 'fulfilled')
      agentNames.value = Object.fromEntries(agents.value.map((a) => [a.id, a.display_name || a.agent_key]));
    if (teams.status === 'fulfilled')
      teamNames.value = Object.fromEntries(teams.value.map((t) => [t.id, t.display_name || t.team_key]));
  }

  async function loadRuns() {
    const recent = (await listRuns()).slice(0, RUN_LIMIT);
    const details = await Promise.allSettled(recent.map((r) => getRun(r.id)));
    runs.value = details.flatMap((d) => (d.status === 'fulfilled' ? [d.value] : []));
  }

  let lastRefresh = 0;
  async function refresh() {
    lastRefresh = Date.now();
    const failed = new Set<SourceKey>();
    const jobs: Promise<unknown>[] = [
      ...buckets.value.map((b) =>
        searchSessions({ status: b, root_only: true, limit: SESSION_LIMIT })
          .then((res) => sessions.set(b, res.items))
          .catch(() => failed.add('sessions')),
      ),
      Promise.all([listOfficeTasks('open', 100, 'delegate'), listOfficeTasks('done', 50, 'delegate')])
        .then(([open, done]) => (officeTasks.value = [...open, ...done]))
        .catch(() => failed.add('office')),
      listCodingTasks(CODING_LIMIT)
        .then((rows) => (codingTasks.value = rows))
        .catch(() => failed.add('coding')),
    ];
    if (canFleet.value) jobs.push(loadRuns().catch(() => failed.add('fleet')));
    await Promise.all(jobs);
    errors.value = [...failed];
    loaded.value = true;
  }

  /** 切到已归档 / 回收站视图或开始搜索时，补读那部分会话 */
  async function ensureBucket(bucket: Bucket) {
    if (buckets.value.includes(bucket)) return;
    buckets.value = [...buckets.value, bucket];
    try {
      const res = await searchSessions({ status: bucket, root_only: true, limit: SESSION_LIMIT });
      sessions.set(bucket, res.items);
    } catch {
      errors.value = [...new Set([...errors.value, 'sessions' as SourceKey])];
    }
  }

  // ——— 操作：按执行的来源分别落到各自的接口 ———
  async function each(list: ConvTask[], fn: (e: Execution) => Promise<boolean> | boolean): Promise<ActionResult> {
    const result: ActionResult = { done: 0, skipped: 0, failed: 0 };
    for (const e of list.flatMap(flat)) {
      try {
        if (await fn(e)) result.done++;
        else result.skipped++;
      } catch {
        result.failed++;
      }
    }
    await refresh();
    return result;
  }

  const live = (e: Execution) => e.status === 'running' || e.status === 'waiting';

  /** 归档：本平台会话归档（还在跑的不行）；编码任务验收为「归档」（还在跑的不行）；运行台终端不归档 */
  const archive = (list: ConvTask[]) =>
    each(list, async (e) => {
      if (e.bucket !== 'active' || live(e)) return false;
      if (e.kind === 'session') await archiveSession(e.refId);
      else if (e.kind === 'bridge') await reviewCodingTask(e.refId, 'archive');
      else return false;
      return true;
    });

  /** 取消归档 / 从回收站恢复：只有本平台会话能恢复（同一个接口两种都管） */
  const restore = (list: ConvTask[]) =>
    each(list, async (e) => {
      if (e.kind !== 'session' || e.bucket === 'active') return false;
      await restoreSession(e.refId);
      return true;
    });

  /** 删除：本平台会话进回收站；运行台终端关闭（在跑的会被结束）；编码任务没有删除接口，跳过 */
  const remove = (list: ConvTask[]) =>
    each(list, async (e) => {
      if (e.kind === 'session' && e.bucket !== 'deleted') await deleteSession(e.refId);
      else if (e.kind === 'terminal') await closeTerminal(e.refId);
      else return false;
      return true;
    });

  /** 收藏跟着任务里第一段本平台会话走（其他来源没有收藏字段） */
  async function togglePin(task: ConvTask): Promise<boolean> {
    const s = flat(task).find((e) => e.kind === 'session');
    if (!s) return false;
    await (s.pinned ? unpinSession(s.refId) : pinSession(s.refId));
    await refresh();
    return true;
  }

  /** 验收通过：交办登记置为完成（没登记成的记在资料袋上）；编码任务一并验收 */
  async function approve(task: ConvTask) {
    const jobs: Promise<unknown>[] = [];
    if (task.officeId) jobs.push(setOfficeTaskStatus(task.officeId, 'done'));
    else if (task.recordId) {
      const id = task.recordId;
      jobs.push(
        getSession(id).then((s) => updateSessionMetadata(id, mergeMeta(s.metadata_json, { cc_reviewed: true }))),
      );
    }
    for (const e of flat(task))
      if (e.kind === 'bridge' && e.status === 'done')
        jobs.push(reviewCodingTask(e.refId, 'approve').catch(() => undefined)); // 已取消 / 已验收的会被拒，不算失败
    await Promise.all(jobs);
    await refresh();
  }

  async function stop(e: Execution) {
    if (e.kind === 'bridge') await cancelCodingTask(e.refId);
    await refresh();
  }

  // ——— 刷新节奏：30 秒 + 切回页面 + 指令面板做完事 ———
  let timer: ReturnType<typeof setInterval> | null = null;
  let polling = false;
  // 切回页面时刷新，但 10 秒内刚刷过就不再刷（有的环境会频繁切换可见状态）
  const onVisible = () => {
    if (!document.hidden && Date.now() - lastRefresh > 10_000) void refresh();
  };
  const onCommandDone = () => void refresh();
  onMounted(() => {
    void loadNames();
    void refresh();
    if (canFleet.value) {
      fleet.startPolling();
      polling = true;
    }
    timer = setInterval(() => {
      if (!document.hidden) void refresh();
    }, POLL_MS);
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener(COMMAND_DONE_EVENT, onCommandDone);
  });
  onUnmounted(() => {
    if (timer) clearInterval(timer);
    if (polling) fleet.stopPolling();
    document.removeEventListener('visibilitychange', onVisible);
    window.removeEventListener(COMMAND_DONE_EVENT, onCommandDone);
  });

  return {
    tasks,
    agentNames,
    teamNames,
    loaded,
    errors,
    canFleet,
    presets: computed(() => fleet.state.presets),
    workspaces: computed(() => fleet.state.workspaces),
    refresh,
    ensureBucket,
    archive,
    restore,
    remove,
    togglePin,
    approve,
    stop,
    focusTerminal: fleet.focusTerminal,
  };
}
