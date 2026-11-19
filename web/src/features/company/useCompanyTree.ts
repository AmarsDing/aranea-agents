/**
 * 公司树的数据编排：组织树 + Agent 名册是骨架（任一读不到就没有树）；
 * 收件箱、交办登记、定时任务是挂在骨架上的补充，各自失败只降级、不拦树。
 */
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { listAgents } from '../agents/api';
import { listInbox } from '../approval/api';
import { listCronTasks, parseCronMetadata } from '../cron/api';
import { listOfficeTasks } from '../office/api';
import { createOrganizationService } from '../../services';
import {
  buildCompanyTree,
  type AgentInput,
  type CronInput,
  type DelegatedInput,
  type InboxInput,
  type OrgTreeInput,
} from './tree';

export type TreeStatus = 'loading' | 'ok' | 'error';
/** 读取失败的补充来源（页面据此说明哪些数字不可信） */
export type PartialSource = 'inbox' | 'delegated' | 'cron';

const POLL_INTERVAL_MS = 60_000;
const COMPANY_KEY = 'aranea:company-tree:company:v1';

function loadCompanyId(): string {
  try {
    return localStorage.getItem(COMPANY_KEY) ?? '';
  } catch {
    return '';
  }
}

export function useCompanyTree() {
  const status = ref<TreeStatus>('loading');
  const partial = ref<PartialSource[]>([]);
  const updatedAt = ref('');
  const companyId = ref(loadCompanyId());
  const nowIso = ref(new Date().toISOString());

  const orgTree = ref<OrgTreeInput[]>([]);
  const agents = ref<AgentInput[]>([]);
  const inbox = ref<InboxInput[]>([]);
  const inboxTotal = ref(0);
  const delegated = ref<DelegatedInput[]>([]);
  const cron = ref<CronInput[]>([]);

  let timer: ReturnType<typeof setInterval> | null = null;

  async function refresh(): Promise<void> {
    const [orgRes, agentRes, inboxRes, delegatedRes, cronRes] = await Promise.allSettled([
      createOrganizationService().ListOrganizationTree({}),
      listAgents(),
      listInbox({ pageSize: 200 }),
      listOfficeTasks('open', 100, 'delegate'),
      listCronTasks(),
    ]);
    nowIso.value = new Date().toISOString();
    if (orgRes.status === 'rejected' || agentRes.status === 'rejected') {
      console.warn('[company] org tree or agents load failed', orgRes, agentRes);
      // 已有旧树时保留旧树，只有首次就失败才显示读取失败。
      if (status.value !== 'ok') status.value = 'error';
      return;
    }
    orgTree.value = (orgRes.value.items ?? []) as OrgTreeInput[];
    agents.value = agentRes.value;
    const failed: PartialSource[] = [];
    if (inboxRes.status === 'fulfilled') {
      inbox.value = inboxRes.value.items;
      inboxTotal.value = inboxRes.value.total;
    } else failed.push('inbox');
    if (delegatedRes.status === 'fulfilled') delegated.value = delegatedRes.value;
    else failed.push('delegated');
    if (cronRes.status === 'fulfilled') {
      cron.value = cronRes.value
        .filter((row) => row.enabled && row.agent_id)
        .map((row) => ({ agentId: row.agent_id, nextRunAt: parseCronMetadata(row).next_run_at ?? '' }));
    } else failed.push('cron');
    partial.value = failed;
    updatedAt.value = nowIso.value;
    status.value = 'ok';
  }

  const tree = computed(() =>
    buildCompanyTree({
      orgTree: orgTree.value,
      agents: agents.value,
      inbox: inbox.value,
      inboxTotal: inboxTotal.value,
      delegated: delegated.value,
      cron: cron.value,
      companyId: companyId.value,
      now: new Date(nowIso.value),
    }),
  );

  watch(companyId, (id) => {
    try {
      localStorage.setItem(COMPANY_KEY, id);
    } catch {
      // 存不了不影响切换
    }
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

  onUnmounted(() => {
    if (timer) clearInterval(timer);
    document.removeEventListener('visibilitychange', onVisibility);
  });

  return { status, partial, updatedAt, nowIso, companyId, tree, refresh };
}
