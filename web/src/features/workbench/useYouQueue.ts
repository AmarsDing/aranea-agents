/**
 * YOU 金门 lane 数据源：服务端统一收件箱（/v1/inbox）。
 *
 * 待办的聚合、配对、去重、权限过滤与排序全部在服务端完成（internal/biz/inbox
 * + internal/service/inbox_sources*.go），本 composable 只做三件事：
 *   - 读：ListInbox 一次取回全部来源，映射成五条 lane（logic.buildLanesFromInbox）；
 *   - 写：DecideInboxItem 统一决议，服务端按 id 回源解析后分发到各领域写路径；
 *   - 刷新：30s 轮询 + 写成功直刷 + WS confirm 事件直刷。
 */
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { decideInboxItem, isConflictError, listInbox, type InboxAction } from '../approval/api';
import { useNavStore } from '../../stores/nav';
import { buildLanesFromInbox } from './logic';
import type { SignReceipt, YouItem, YouLaneId, YouLaneState } from './types';

const POLL_INTERVAL_MS = 30_000;
/** WS confirm 事件直刷节流（事件驱动而非常驻轮询） */
const WS_POKE_MIN_INTERVAL_MS = 5_000;
/** 单次取回上限（服务端单页上限 200；计数走各来源 count，不受此限） */
const INBOX_PAGE_SIZE = 200;

const LANE_ORDER: YouLaneId[] = ['approval', 'trade', 'publish', 'listing', 'alerts'];

const VERB_ACTION: Record<SignReceipt['verb'], InboxAction> = {
  approved: 'approve',
  rejected: 'reject',
  acked: 'ack',
  resolved: 'resolve',
};

function emptyLane(lane: YouLaneId): YouLaneState {
  return { lane, items: [], count: 0, updatedAt: '', degraded: false };
}

export function useYouQueue() {
  const { t } = useI18n();
  const nav = useNavStore();
  const canReview = computed(() => nav.permissions.includes('*') || nav.permissions.includes('approval.review'));

  const lanes = ref<YouLaneState[]>(LANE_ORDER.map(emptyLane));
  const loading = ref(false);
  /** 条目是否要求驳回理由（服务端声明；仅用于提交前的就地提示，服务端仍会校验） */
  const rejectNeedsNote = new Set<string>();
  /** 签批成功后的乐观消账（来源侧落库可能有秒级延迟；2 分钟内不再渲染该条） */
  const settledUntil = new Map<string, number>();
  let timer: ReturnType<typeof setInterval> | null = null;
  let lastWsPokeAt = 0;
  let refreshing = false;
  let refreshQueued = false;

  const total = computed(() => lanes.value.reduce((acc, l) => acc + l.count, 0));
  /** laneId → 状态 的快取（展示层 O(1) 取） */
  const byLane = computed(() => new Map(lanes.value.map((l) => [l.lane, l])));
  /** 最旧一次成功刷新（信任栏新鲜度；失败 lane 保留旧值，不伪造新鲜） */
  const oldestUpdatedAt = computed(() => {
    const ts = lanes.value
      .map((l) => l.updatedAt)
      .filter(Boolean)
      .sort();
    return ts[0] ?? '';
  });

  async function refresh(): Promise<void> {
    if (refreshing) {
      refreshQueued = true;
      return;
    }
    refreshing = true;
    loading.value = true;
    const prevUpdatedAt = Object.fromEntries(lanes.value.map((l) => [l.lane, l.updatedAt]));
    try {
      const res = await listInbox({ pageSize: INBOX_PAGE_SIZE });
      rejectNeedsNote.clear();
      for (const row of res.items) if (row.reject_requires_note) rejectNeedsNote.add(row.id);
      lanes.value = applySettled(
        buildLanesFromInbox(res.items, res.counts, res.degraded, new Date().toISOString(), prevUpdatedAt),
      );
    } catch (err) {
      // 整体读取失败：保留上次的行与刷新时间，全部 lane 标降级（不清空、不伪造新鲜）
      console.warn('[workbench] inbox refresh failed', err);
      lanes.value = lanes.value.map((l) => ({ ...l, degraded: true }));
    } finally {
      loading.value = false;
      refreshing = false;
      if (refreshQueued) {
        refreshQueued = false;
        void refresh();
      }
    }
  }

  /** 已签条目立即从快照消失（计数同步扣减），避免「签了还在」的假闭环 */
  function applySettled(input: YouLaneState[]): YouLaneState[] {
    const nowMs = Date.now();
    for (const [id, until] of settledUntil) if (until <= nowMs) settledUntil.delete(id);
    if (settledUntil.size === 0) return input;
    return input.map((lane) => {
      const kept = lane.items.filter((item) => !settledUntil.has(item.id));
      const removed = lane.items.length - kept.length;
      if (removed === 0) return lane;
      let agentCounts = lane.agentCounts;
      if (agentCounts) {
        agentCounts = { ...agentCounts };
        for (const item of lane.items) {
          if (!settledUntil.has(item.id) || !item.agentKey) continue;
          const next = (agentCounts[item.agentKey] ?? 0) - 1;
          if (next > 0) agentCounts[item.agentKey] = next;
          else delete agentCounts[item.agentKey];
        }
      }
      return { ...lane, items: kept, count: Math.max(0, lane.count - removed), agentCounts };
    });
  }

  /** WS confirm 事件到达 → 直刷（节流防风暴；轮询仍兜 30s 对账） */
  function pokeFromWs(): void {
    const now = Date.now();
    if (now - lastWsPokeAt < WS_POKE_MIN_INTERVAL_MS) return;
    lastWsPokeAt = now;
    void refresh();
  }

  /**
   * 签收：统一走 DecideInboxItem。失败抛错（行内展示），成功直刷并返回回执。
   * 条目已被他人处理/已失效时服务端返回 409 → 刷新后提示「已被处理」。
   */
  async function sign(item: YouItem, verb: SignReceipt['verb'], note = ''): Promise<SignReceipt | null> {
    if (item.action === 'goto' || item.stale) throw new Error(t('workbench.sign.noContext'));
    const reason = note.trim();
    if (verb === 'rejected' && !reason && rejectNeedsNote.has(item.id)) {
      throw new Error(t('workbench.sign.reasonRequired'));
    }
    try {
      await decideInboxItem(item.id, VERB_ACTION[verb], reason);
    } catch (err) {
      if (isConflictError(err)) {
        await refresh();
        throw new Error(t('workbench.sign.alreadyHandled'), { cause: err });
      }
      throw err;
    }
    const receipt: SignReceipt = {
      itemId: item.id,
      lane: item.lane,
      verb,
      title: item.summary,
      at: new Date().toISOString(),
    };
    settledUntil.set(item.id, Date.now() + 120_000);
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

  // 权限异步下发（MainLayout ensureMenus）：到达/变更后立刻重取，服务端按新权限过滤。
  watch(
    () => nav.permissions.join(','),
    () => void refresh(),
  );

  onUnmounted(() => {
    if (timer) clearInterval(timer);
    document.removeEventListener('visibilitychange', onVisibility);
  });

  return { lanes, byLane, total, oldestUpdatedAt, loading, canReview, refresh, pokeFromWs, sign };
}
