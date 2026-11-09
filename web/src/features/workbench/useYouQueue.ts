/**
 * YOU 金门四 lane 聚合 composable（设计方案 §5.1 / §5.2）。
 *
 * 数据源（现有 API 直读快照，30s 轮询 + 审批动作局部直刷 + WS confirm 直刷）：
 *   ① 交易确认 → decision/v1 ListDecisionRecords(category=hitl_approval，
 *      outcome 过滤词已对齐真实词表：asked=待决 —— W1b 修复恒空 bug，
 *      见需求文档 §5.2 关键缺口②）
 *   ② 发文审批 → review.v1 ListPendingReviews（W1b A-2 收编，经 mvpApi 适配层；
 *      reviewer 服务端取 JWT，前端不再自报）
 *   ③ 上架确认 → review.v1 ListOpportunities（同上收编）
 *   ④ 告警与凭据 → listTaskDeadLetters(pending) + cron-task-runs failure
 *
 * TECH-DEBT: direct API call; move to store — 跨域直读未过 Store。
 * /v1/approvals 聚合端点（W1b A-1）一期只覆盖 task_review/evolution/
 * skill 三类，与本队列四 lane 域不同集；hitl 工具确认（trade）待 A-3 落库、
 * 发文/上架待聚合器扩类后，本 composable 再切 /v1/approvals 单源（W3）。
 */
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { listDecisionRecords } from '../decision/api';
import { mvpApi, type Opportunity, type ReviewTask } from '../mvp/api';
import { listTaskDeadLetters } from '../teams/api';
import { listCronTaskRuns } from '../cron/api';
import { buildAlertsLane, buildListingLane, buildPublishLane, buildTradeLane } from './logic';
import type { SignReceipt, YouItem, YouLaneId, YouLaneState } from './types';

const POLL_INTERVAL_MS = 30_000;
/** WS confirm 事件直刷节流（事件驱动而非常驻轮询） */
const WS_POKE_MIN_INTERVAL_MS = 5_000;

function emptyLane(lane: YouLaneId): YouLaneState {
  return { lane, items: [], count: 0, updatedAt: '', degraded: false };
}

export function useYouQueue() {
  const lanes = ref<YouLaneState[]>([
    emptyLane('trade'),
    emptyLane('publish'),
    emptyLane('listing'),
    emptyLane('alerts'),
  ]);
  const loading = ref(false);
  let timer: ReturnType<typeof setInterval> | null = null;
  let lastWsPokeAt = 0;

  const total = computed(() => lanes.value.reduce((acc, l) => acc + l.count, 0));
  /** laneId → 状态 的快取（展示层 O(1) 取） */
  const byLane = computed(() => new Map(lanes.value.map((l) => [l.lane, l])));
  /** 最旧一次成功刷新（信任栏新鲜度） */
  const oldestUpdatedAt = computed(() => {
    const ts = lanes.value
      .map((l) => l.updatedAt)
      .filter(Boolean)
      .sort();
    return ts[0] ?? '';
  });

  async function refresh(): Promise<void> {
    loading.value = true;
    const [decisionRes, reviewRes, oppRes, deadRes, cronRes] = await Promise.allSettled([
      listDecisionRecords({ category: 'hitl_approval', pageSize: 50 }),
      mvpApi.listReview(),
      mvpApi.listOpp(),
      listTaskDeadLetters({ status: 'pending', limit: 20 }),
      listCronTaskRuns({ status: 'failure', limit: 20 }),
    ]);
    try {
      const now = new Date().toISOString();
      const next: YouLaneState[] = [
        decisionRes.status === 'fulfilled'
          ? buildTradeLane(decisionRes.value.items, now, false)
          : { ...emptyLane('trade'), updatedAt: now, degraded: true },
        reviewRes.status === 'fulfilled'
          ? buildPublishLane(reviewRes.value.items as ReviewTask[], now, false)
          : { ...emptyLane('publish'), updatedAt: now, degraded: true },
        oppRes.status === 'fulfilled'
          ? buildListingLane(oppRes.value.items as Opportunity[], now, false)
          : { ...emptyLane('listing'), updatedAt: now, degraded: true },
        deadRes.status === 'fulfilled' && cronRes.status === 'fulfilled'
          ? buildAlertsLane(deadRes.value, cronRes.value.items, now, false)
          : {
              ...emptyLane('alerts'),
              updatedAt: now,
              degraded: true,
              // 任一域失败仍尽力显示另一半（不打正常高分；计数只含成功域）
              ...(deadRes.status === 'fulfilled' || cronRes.status === 'fulfilled'
                ? buildAlertsLane(
                    deadRes.status === 'fulfilled' ? deadRes.value : [],
                    cronRes.status === 'fulfilled' ? cronRes.value.items : [],
                    now,
                    true,
                  )
                : {}),
            },
      ];
      lanes.value = next;
    } finally {
      loading.value = false;
    }
  }

  /** WS confirm 事件到达 → 直刷（节流防风暴；轮询仍兜 30s 对账） */
  function pokeFromWs(): void {
    const now = Date.now();
    if (now - lastWsPokeAt < WS_POKE_MIN_INTERVAL_MS) return;
    lastWsPokeAt = now;
    void refresh();
  }

  /**
   * 签收写路径。仅 publish（approve/reject）与 listing（ack）可写；
   * trade/alerts lane 只下钻（诚实口径：无写路径不假装可签）。
   * 成功后局部直刷并返回回执（驱动落款戳 + 绿动词日志行）。
   */
  async function sign(item: YouItem, verb: 'approved' | 'rejected' | 'acked', note = ''): Promise<SignReceipt | null> {
    if (item.action === 'goto') return null;
    let ok = false;
    if (item.lane === 'publish') {
      const id = item.id.replace(/^review:/, '');
      if (verb === 'approved') {
        await mvpApi.approve(id, note || 'ok');
        ok = true;
      } else if (verb === 'rejected') {
        await mvpApi.reject(id, note || 'reject');
        ok = true;
      }
    } else if (item.lane === 'listing' && verb === 'acked') {
      await mvpApi.ackOpp(item.id.replace(/^opp:/, ''));
      ok = true;
    }
    if (!ok) return null;
    const receipt: SignReceipt = {
      itemId: item.id,
      lane: item.lane,
      verb,
      title: item.summary,
      at: new Date().toISOString(),
    };
    await refresh();
    return receipt;
  }

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

  return { lanes, byLane, total, oldestUpdatedAt, loading, refresh, pokeFromWs, sign };
}
