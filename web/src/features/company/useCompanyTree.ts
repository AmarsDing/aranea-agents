/**
 * 看板总览的数据编排：组织树 + Agent 名册是骨架（任一读不到就没有图）；
 * 收件箱、交办登记（及其执行态）、定时任务、今日用量、经营信号是挂在骨架上的补充，
 * 各自失败只降级、不拦图。先试后端 GET /v1/ceoboard/org-graph 一次拼好的图（含总经理巡视 / 汇报），
 * 读不到（含旧后端 404）才在前端拼。
 */
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { listAgentsPaged } from '../agents/api';
import { listInbox } from '../approval/api';
import { fetchCompanySnapshot, fetchOrgGraph } from '../ceoboard/company';
import type { OpsAnomaly } from '../ceoboard/types';
import { listCronTasks, parseCronMetadata } from '../cron/api';
import { listOfficeTasks } from '../office/api';
import { listTaskSituations } from '../session/api';
import { classifyOfficeDelegated } from '../today/logic';
import { listTopAgentUsage } from '../usage/api';
import { createOrganizationService } from '../../services';
import {
  buildBoard,
  GM_REPORT_SOURCE,
  type AgentInput,
  type Board,
  type CronInput,
  type InboxInput,
  type OrgTreeInput,
  type PartialSource,
  type UsageInput,
  type WorkInput,
} from './tree';

export type TreeStatus = 'loading' | 'ok' | 'error';

const POLL_INTERVAL_MS = 60_000;
// 切回页面时的刷新闸：内置浏览器会频繁切换 visibility，避免每两三秒打一轮请求
const MIN_REFRESH_GAP_MS = 15_000;
const FETCH_LIMIT = 100;
const AGENT_PAGE = 500;
// shortcut: 今日用量只取花费前 200 个 Agent（接口上限）；今天跑过的 Agent 超过 200 个时再改成分页
const USAGE_LIMIT = 200;

/** 整个名册（接口一页最多 500 个）；只取第一页时多数 Agent 不会出现在图上。 */
async function loadAllAgents(): Promise<AgentInput[]> {
  const out: AgentInput[] = [];
  for (let offset = 0; ; offset += AGENT_PAGE) {
    const page = await listAgentsPaged({ limit: AGENT_PAGE, offset });
    out.push(...page.items);
    if (page.items.length < AGENT_PAGE || out.length >= page.total) return out;
  }
}

async function loadWork(): Promise<WorkInput[]> {
  const tasks = await listOfficeTasks('open', FETCH_LIMIT, 'delegate');
  if (tasks.length === 0) return [];
  // 执行态读不到时按「在做」处理（与工作台交办列表同一口径）
  const situations = await listTaskSituations({ limit: FETCH_LIMIT }).then(
    (page) => page.items,
    (err: unknown) => {
      console.warn('[company] task situations load failed', err);
      return [];
    },
  );
  const byId = new Map(tasks.map((task) => [task.id, task]));
  return classifyOfficeDelegated(tasks, situations).map((row) => {
    const task = byId.get(row.officeTaskId ?? '');
    return {
      id: row.id,
      title: row.title,
      assignee_kind: task?.assignee_kind ?? '',
      assignee_id: task?.assignee_id ?? '',
      state: row.state,
      at: row.at,
      to: row.to,
    };
  });
}

async function loadUsage(): Promise<UsageInput[]> {
  const rows = await listTopAgentUsage({ range: 'today', limit: USAGE_LIMIT });
  return rows
    .filter((row) => row.agent_id)
    .map((row) => ({ agentId: row.agent_id, calls: row.call_count, costMicro: row.total_cost_micro_usd }));
}

export function useCompanyTree() {
  const status = ref<TreeStatus>('loading');
  const partial = ref<PartialSource[]>([]);
  const nowIso = ref(new Date().toISOString());
  /** 后端 org-graph 给的整张图；为 null 时用下面的七个来源在前端拼 */
  const graph = ref<Board | null>(null);

  const orgTree = ref<OrgTreeInput[]>([]);
  const agents = ref<AgentInput[]>([]);
  const inbox = ref<InboxInput[]>([]);
  const inboxTotal = ref(0);
  const work = ref<WorkInput[]>([]);
  const cron = ref<CronInput[]>([]);
  const usage = ref<UsageInput[]>([]);
  const signals = ref<OpsAnomaly[]>([]);

  let timer: ReturnType<typeof setInterval> | null = null;
  let lastRefresh = 0;

  async function refresh(): Promise<void> {
    lastRefresh = Date.now();
    // 后端一次拼好的图（含总经理巡视 / 汇报）优先；读不到（含旧后端 404）回落到下面的七接口拼装。
    const [graphRes, signalRes] = await Promise.allSettled([fetchOrgGraph(), fetchCompanySnapshot()]);
    if (signalRes.status === 'fulfilled') signals.value = signalRes.value.anomalies ?? [];
    if (graphRes.status === 'fulfilled') {
      const { companies, me, partial: graphPartial } = graphRes.value;
      graph.value = { companies, me };
      nowIso.value = new Date().toISOString();
      partial.value = [...(graphPartial ?? []), ...(signalRes.status === 'rejected' ? (['signals'] as const) : [])];
      status.value = 'ok';
      return;
    }
    console.warn('[company] org graph unavailable, assembling in the browser', graphRes.reason);
    graph.value = null;
    await assemble(signalRes.status === 'fulfilled');
  }

  async function assemble(signalsOk: boolean): Promise<void> {
    const [orgRes, agentRes, inboxRes, workRes, cronRes, usageRes] = await Promise.allSettled([
      createOrganizationService().ListOrganizationTree({}),
      loadAllAgents(),
      listInbox({ pageSize: 200 }),
      loadWork(),
      listCronTasks(),
      loadUsage(),
    ]);
    nowIso.value = new Date().toISOString();
    if (orgRes.status === 'rejected' || agentRes.status === 'rejected') {
      console.warn('[company] org tree or agents load failed', orgRes, agentRes);
      // 已有旧图时保留旧图，只有首次就失败才显示读取失败。
      if (status.value !== 'ok') status.value = 'error';
      return;
    }
    orgTree.value = (orgRes.value.items ?? []) as OrgTreeInput[];
    agents.value = agentRes.value;
    const failed: PartialSource[] = [];
    if (inboxRes.status === 'fulfilled') {
      inbox.value = inboxRes.value.items.filter((item) => item.source !== GM_REPORT_SOURCE);
      inboxTotal.value = inboxRes.value.total - (inboxRes.value.counts[GM_REPORT_SOURCE] ?? 0);
    } else failed.push('inbox');
    if (workRes.status === 'fulfilled') work.value = workRes.value;
    else failed.push('delegated');
    if (cronRes.status === 'fulfilled') {
      cron.value = cronRes.value
        .filter((row) => row.enabled && row.agent_id)
        .map((row) => ({ agentId: row.agent_id, nextRunAt: parseCronMetadata(row).next_run_at ?? '' }));
    } else failed.push('cron');
    if (usageRes.status === 'fulfilled') usage.value = usageRes.value;
    else failed.push('usage');
    if (!signalsOk) failed.push('signals');
    partial.value = failed;
    status.value = 'ok';
  }

  const board = computed(
    () =>
      graph.value ??
      buildBoard({
        orgTree: orgTree.value,
        agents: agents.value,
        inbox: inbox.value,
        inboxTotal: inboxTotal.value,
        work: work.value,
        cron: cron.value,
        usage: usage.value,
        now: new Date(nowIso.value),
      }),
  );

  function onVisibility() {
    if (!document.hidden && Date.now() - lastRefresh > MIN_REFRESH_GAP_MS) void refresh();
  }

  onMounted(() => {
    void refresh();
    timer = setInterval(() => {
      if (!document.hidden) void refresh();
    }, POLL_INTERVAL_MS);
    document.addEventListener('visibilitychange', onVisibility);
  });

  onUnmounted(() => {
    if (timer) clearInterval(timer);
    document.removeEventListener('visibilitychange', onVisibility);
  });

  return { status, partial, nowIso, board, signals, refresh };
}
