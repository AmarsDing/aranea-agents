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
import { groupRoster, isPendingDecision, decisionAgentKey } from './logic';
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

/** 信任六态裁决（R1/R5 同源；回放 P2 启用，P0 恒不命中）
 *  失败（degraded）必须优先于积压（backlog）暴露，否则拉取失败会被 backlog 掩盖成"正常"。 */
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
  if (args.lanesDegraded) return 'degraded';
  if (args.backlogCount > 0) return 'backlog';
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
  /** 今日人签决策按业务线归因（无法归因不计入任何线，不摊派） */
  const doneByLine = ref<Partial<Record<BizLineId, number>>>({});

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
          org_node_id: a.taxonomy_position_id,
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
      const perLine: Partial<Record<BizLineId, number>> = {};
      const lineOfAgent = new Map<string, BizLineId>();
      for (const a of allAgents.value) {
        if (a.bizLine) lineOfAgent.set(a.key, a.bizLine);
      }
      for (const d of res.items) {
        if (isPendingDecision(d.outcome)) continue;
        const isHuman = (d.actor_type ?? '').toLowerCase() === 'human';
        if (isHuman) {
          humanDone += 1;
          const line = lineOfAgent.get(decisionAgentKey(d));
          if (line) perLine[line] = (perLine[line] ?? 0) + 1;
        }
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
      doneByLine.value = perLine;
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

  /**
   * 四线卡网格 + SOURCE 权限卡共用的证据态。
   * 口径：connected = 该线有在岗 agent 且花名册加载成功；pending 只算可归因来源；
   * done = 今日人签决策中能归因到该线的条数（无法归因不摊派，无数据显示 —）。
   */
  function buildBizLineCards(lanes: readonly YouLaneState[]): BizLineCard[] {
    const pendingByLine = new Map<BizLineId, number>();
    const add = (line: BizLineId | '', n: number) => {
      if (!line || n <= 0) return;
      pendingByLine.set(line, (pendingByLine.get(line) ?? 0) + n);
    };
    const lineOfAgent = new Map<string, BizLineId>();
    for (const a of allAgents.value) if (a.bizLine) lineOfAgent.set(a.key, a.bizLine);

    const laneOf = (lane: string) => lanes.find((l) => l.lane === lane);
    const trade = laneOf('trade');
    for (const [agentKey, n] of Object.entries(trade?.agentCounts ?? {})) {
      add(lineOfAgent.get(agentKey) ?? '', Number(n) || 0);
    }
    // 审批中心条目按后端 agent_key 归因；无法归因的行不进任何业务线（不摊派）
    const approval = laneOf('approval');
    for (const [agentKey, n] of Object.entries(approval?.agentCounts ?? {})) {
      add(lineOfAgent.get(agentKey) ?? '', Number(n) || 0);
    }
    add('hotcraft', laneOf('publish')?.count ?? 0);
    add('ecmove', laneOf('listing')?.count ?? 0);

    return BIZ_LINES.map((id) => ({
      id,
      connected: !rosterFailed.value && allAgents.value.some((a) => a.bizLine === id),
      pending: pendingByLine.get(id) ?? 0,
      done: doneByLine.value[id] ?? 0,
      note: '',
    }));
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
    doneByLine,
    loadRoster,
    fetchDecisionBackfill,
    openDrill,
    closeDrill,
    buildBizLineCards,
    deriveTrustState,
  };
}
