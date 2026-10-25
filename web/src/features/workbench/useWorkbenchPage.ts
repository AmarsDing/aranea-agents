/**
 * 作战总台页面状态机：花名册 / 业务线卡片 / 信任态 / 下钻抽屉 / 冷开回填。
 * 数据流铁律：页面只组装本 composable + useYouQueue + useWorkbenchStream；
 * 本 composable 直接持有跨域读取（TECH-DEBT 见 useYouQueue 头注）。
 */
import { computed, ref } from 'vue';
import { createOrganizationService } from '../../services';
import type { OrganizationTreeNode } from '../../services/kratos/organization/v1/index';
import { listAgents } from '../agents/api';
import { listDecisionRecords } from '../decision/api';
import { groupRoster, isPendingDecision } from './logic';
import type { BizLineCard, BizLineId, DrillTarget, RosterModel, TrustState, YouLaneState } from './types';

const BIZ_LINES: BizLineId[] = ['novatrader', 'hotcraft', 'assistant', 'ecmove'];

function flattenOrgTree(nodes: readonly OrganizationTreeNode[]): {
  departments: { id: string; name: string }[];
  positions: { id: string; name: string; parentId: string }[];
} {
  const departments: { id: string; name: string }[] = [];
  const positions: { id: string; name: string; parentId: string }[] = [];
  const walk = (list: readonly OrganizationTreeNode[], parentId: string) => {
    for (const n of list) {
      const node = n.node;
      if (!node?.id) continue;
      const level = (node.level ?? '').toLowerCase();
      if (level === 'department') {
        departments.push({ id: node.id, name: node.name ?? '' });
      } else if (level === 'position') {
        positions.push({ id: node.id, name: node.name ?? '', parentId });
      }
      walk(n.children ?? [], level === 'department' ? node.id : parentId);
    }
  };
  walk(nodes, '');
  return { departments, positions };
}

/** 信任六态裁决（R1/R5 同源；回放 P2 启用，P0 恒不命中） */
export function deriveTrustState(args: {
  wsConnected: boolean;
  staleMs: number;
  lanesDegraded: boolean;
  backlogCount: number;
  replayMode: boolean;
  wsStaleMs?: number;
}): TrustState {
  if (args.replayMode) return 'replay';
  if (!args.wsConnected) return 'offline';
  if (args.staleMs > (args.wsStaleMs ?? 5 * 60_000)) return 'stale';
  if (args.backlogCount > 0) return 'backlog';
  if (args.lanesDegraded) return 'degraded';
  return 'live';
}

export function useWorkbenchPage() {
  const roster = ref<RosterModel>({ depts: [], unassigned: [] });
  const rosterLoaded = ref(false);
  const rosterFailed = ref(false);
  const drill = ref<DrillTarget>(null);
  const backfillDone = ref(false);
  const backfillFailed = ref(false);
  const humanDoneToday = ref(0);

  async function loadRoster(): Promise<void> {
    try {
      const svc = createOrganizationService();
      const [treeRes, agents] = await Promise.all([svc.ListOrganizationTree({}), listAgents()]);
      const { departments, positions } = flattenOrgTree(treeRes.items ?? []);
      roster.value = groupRoster(
        departments,
        positions,
        agents.map((a) => ({
          agent_key: a.agent_key,
          display_name: a.display_name,
          icon: a.icon,
          org_node_id: a.org_node_id,
          deleted_at: a.deleted_at,
        })),
      );
      rosterFailed.value = false;
    } catch {
      // 拉取失败：花名册空渲染 + 信任态 degraded（不打正常高分）
      rosterFailed.value = true;
    } finally {
      rosterLoaded.value = true;
    }
  }

  /** 全部专项（扁平，角色行渲染序：花名册内 → 未编组） */
  const allAgents = computed(() => {
    const rows = roster.value.depts.flatMap((d) => [...d.positions.flatMap((p) => p.agents), ...d.floatAgents]);
    return [...rows, ...roster.value.unassigned];
  });

  /**
   * 冷开回填（验收红线 3：30s 内总览/YOU 非空）。
   * decision 时间窗（今日零点起）：pending → YOU lane 已兜（useYouQueue 全量类别过滤），
   * 非 pending 且 actor_type=human → 『你』绿动词行（刷新后落点仍在）；
   * 其余 → agent 行。返回供 stream.backfill 注入的行。
   */
  async function fetchDecisionBackfill(): Promise<{ lines: import('./types').LogLine[]; humanDoneCount: number }> {
    const empty = { lines: [] as import('./types').LogLine[], humanDoneCount: 0 };
    const dayStart = new Date();
    dayStart.setHours(0, 0, 0, 0);
    try {
      const res = await listDecisionRecords({
        time_from: dayStart.toISOString(),
        pageSize: 100,
      });
      const lines: import('./types').LogLine[] = [];
      let humanDone = 0;
      for (const d of res.items) {
        if (isPendingDecision(d.outcome)) continue;
        const isHuman = (d.actor_type ?? '').toLowerCase() === 'human';
        if (isHuman) humanDone += 1;
        lines.push({
          id: `dec:${d.decision_key}`,
          at: d.created_at ?? dayStart.toISOString(),
          kind: isHuman ? 'you' : 'agent',
          agentKey: d.actor_key ?? '',
          text: `${d.actor_key || '?'} · ${d.scenario || d.category || 'decision'} · ${d.outcome || 'done'}`.slice(
            0,
            96,
          ),
        });
      }
      lines.sort((a, b) => b.at.localeCompare(a.at));
      humanDoneToday.value = humanDone;
      backfillFailed.value = false;
      backfillDone.value = true;
      return { lines: lines.slice(0, 60), humanDoneCount: humanDone };
    } catch (err) {
      // 回填失败：KPI 维持待回填（done 亮 —），信任态 degraded；用户提示由 axios 拦截统一承载
      console.warn('[workbench] decision backfill failed', err);
      backfillFailed.value = true;
      return empty;
    }
  }

  function openDrill(target: Exclude<DrillTarget, null>): void {
    drill.value = target;
  }

  function closeDrill(): void {
    drill.value = null;
  }

  /** 四线卡网格 + SOURCE 权限卡共用的证据态（无后端证据整卡『待接入』） */
  function buildBizLineCards(lanes: readonly YouLaneState[]): BizLineCard[] {
    const pendingByLine = new Map<BizLineId, number>();
    // 归因口径：decision trade lane 项已带 agentKey——但 lane 不区业务线；
    // P0 诚实口径：卡片 pending 只挂可归因 lane（hotcraft=publish、ecmove=listing），
    // novatrader 后端证据为零 → 整卡待接入（验收红线 1），assistant=alerts lane 计数。
    const laneOf = (lane: string) => lanes.find((l) => l.lane === lane);
    pendingByLine.set('hotcraft', laneOf('publish')?.count ?? 0);
    pendingByLine.set('ecmove', laneOf('listing')?.count ?? 0);
    pendingByLine.set('assistant', laneOf('alerts')?.count ?? 0);
    return BIZ_LINES.map((id) => {
      const hasAgents = allAgents.value.some((a) => a.bizLine === id);
      const connected = id !== 'novatrader' && hasAgents;
      return {
        id,
        connected,
        pending: pendingByLine.get(id) ?? 0,
        done: 0,
        note: '',
      } satisfies BizLineCard;
    });
  }

  return {
    roster,
    rosterLoaded,
    rosterFailed,
    allAgents,
    drill,
    backfillDone,
    backfillFailed,
    humanDoneToday,
    loadRoster,
    fetchDecisionBackfill,
    openDrill,
    closeDrill,
    buildBizLineCards,
    deriveTrustState,
  };
}
