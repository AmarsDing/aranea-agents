/**
 * YOU 金门四 lane 聚合 composable（设计方案 §5.1 / §5.2）。
 *
 * 数据源（现有 API 直读快照，30s 轮询 + 审批动作局部直刷 + WS confirm 直刷）：
 *   ① 交易确认 → decision/v1 ListDecisionRecords(category=hitl_approval，outcome 客户端过滤)
 *      —— 决策记录无写路径（写在 team run 域），行内只下钻，诚实口径见设计 §5.1
 *   ② 发文审批 → mvp listReview / approve / reject
 *   ③ 上架确认 → mvp listOpp / ackOpp
 *   ④ 告警与凭据 → listTaskDeadLetters(pending) + cron-task-runs failure
 *
 * TECH-DEBT: direct API call; move to store — 跨域直读未过 Store，
 * 待后端 GET /v1/monitor/attention-summary 聚合端点上线后切换（先例：useAttentionBoard）。
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
