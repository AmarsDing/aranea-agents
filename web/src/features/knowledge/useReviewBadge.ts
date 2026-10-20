// 审查待办徽章（SP3-P3）：治理提案 + 写回待审双计数。
// 拉取口径与 useWorkbenchCommands 的 reviewGovernance / reviewWriteBackPending 对话框一致：
// 治理取当前库 pending、为空则回退写回 home；写回只统计写回 home（无 home 记 0，对齐命令「home missing」口径）。
// 数据流纪律：一次性状态直调 features/knowledge/api（同 useWorkbenchCommands TECH-DEBT(FL5) 先例），
// fetcher 经构造注入供单测替换；单项失败只把该项置 null、不拖垮另一项，徽章永不炸 UI（静默降级）。
import { computed, getCurrentInstance, onMounted, onUnmounted, ref, watch, type Ref } from 'vue';
import { getWriteBackHome, listGovernanceProposals, listWriteBackPending } from './api';

/** 轮询间隔（60s）；窗口聚焦时额外立即刷新一次。 */
export const REVIEW_BADGE_POLL_INTERVAL_MS = 60_000;

export interface ReviewBadgeCounts {
  /** 治理提案待审数；null = 拉取失败（未知，UI 显示「—」）。 */
  governancePending: number | null;
  /** 写回待审数；null = 拉取失败（未知）；无 home 记 0。 */
  writebackPending: number | null;
}

/** 拉取依赖（构造注入；默认 features/knowledge/api 既有客户端函数）。
 *  G-30：治理返回 {items,total} 分页对象——角标取 total（同口径总量，不再被页大小截断）。 */
export interface ReviewBadgeFetchers {
  listGovernancePending: (collectionId: string) => Promise<{ items: unknown[]; total: number }>;
  listWriteBackPending: (collectionId: string) => Promise<unknown[]>;
  getWriteBackHome: () => Promise<{ found: boolean; collection_id: string }>;
}

const defaultFetchers: ReviewBadgeFetchers = {
  listGovernancePending: (collectionId) => listGovernanceProposals(collectionId, 'pending'),
  listWriteBackPending: (collectionId) => listWriteBackPending(collectionId),
  getWriteBackHome: () => getWriteBackHome(),
};

/**
 * 拉取一次双计数（纯流程、无 Vue 依赖，可独立单测）。
 * 各项独立结算、永不抛错：失败项置 null（UI 以「—」占位，total 按 0 计入）。
 */
export async function fetchReviewBadgeCounts(
  vaultId: string,
  fetchers: ReviewBadgeFetchers = defaultFetchers,
): Promise<ReviewBadgeCounts> {
  // home 先行：治理回退与写回吐向都以它为口径；home 自身失败时写回只能记未知（null）。
  let homeId = '';
  let homeFailed = false;
  try {
    const home = await fetchers.getWriteBackHome();
    if (home.found && home.collection_id) homeId = home.collection_id;
  } catch {
    homeFailed = true;
  }

  const [governancePending, writebackPending] = await Promise.all([
    (async (): Promise<number | null> => {
      try {
        let res = await fetchers.listGovernancePending(vaultId);
        if (res.total === 0 && homeId && homeId !== vaultId) {
          res = await fetchers.listGovernancePending(homeId);
        }
        return res.total;
      } catch {
        return null;
      }
    })(),
    (async (): Promise<number | null> => {
      if (homeFailed) return null;
      if (!homeId) return 0; // 无写回 home → 对话框口径为无可审条目，计 0
      try {
        return (await fetchers.listWriteBackPending(homeId)).length;
      } catch {
        return null;
      }
    })(),
  ]);
  return { governancePending, writebackPending };
}

export interface UseReviewBadgeOptions {
  fetchers?: Partial<ReviewBadgeFetchers>;
  intervalMs?: number;
}

export function useReviewBadge(vaultId: Ref<string>, opts?: UseReviewBadgeOptions) {
  const fetchers: ReviewBadgeFetchers = { ...defaultFetchers, ...opts?.fetchers };
  const intervalMs = opts?.intervalMs ?? REVIEW_BADGE_POLL_INTERVAL_MS;

  const counts = ref<ReviewBadgeCounts>({ governancePending: null, writebackPending: null });
  const lastRefreshAt = ref<number | null>(null);
  /** 总数（未知项按 0 计）：>0 时 UI 亮角标；全 null 时隐藏角标（静默降级不炸 UI）。 */
  const total = computed(() => (counts.value.governancePending ?? 0) + (counts.value.writebackPending ?? 0));

  let timer: ReturnType<typeof setInterval> | undefined;
  let seq = 0; // 代际守卫：慢响应不覆盖新结果；stop 后在途响应落地即丢弃
  let running = false;

  async function refresh() {
    const my = ++seq;
    const vault = vaultId.value;
    if (!vault) {
      counts.value = { governancePending: null, writebackPending: null };
      lastRefreshAt.value = null;
      return;
    }
    const next = await fetchReviewBadgeCounts(vault, fetchers);
    if (my !== seq) return;
    counts.value = next;
    lastRefreshAt.value = Date.now();
  }

  function onWindowFocus() {
    void refresh();
  }

  function start() {
    if (running) return; // 幂等：重复 start 只保留一个计时器/监听
    running = true;
    void refresh();
    timer = setInterval(() => void refresh(), intervalMs);
    window.addEventListener('focus', onWindowFocus);
  }

  function stop() {
    running = false;
    seq += 1; // 在途响应丢弃，stop 后不再落地任何结果
    if (timer !== undefined) {
      clearInterval(timer);
      timer = undefined;
    }
    window.removeEventListener('focus', onWindowFocus);
  }

  // vault 切换：运行中立即按新库重取；未启动（setup 期）不发起请求。
  watch(vaultId, () => {
    if (running) void refresh();
  });

  // 组件上下文自动跟随挂载/卸载；非组件上下文（单测）手工 start/stop，不注册钩子。
  if (getCurrentInstance()) {
    onMounted(start);
    onUnmounted(stop);
  }

  return { counts, total, lastRefreshAt, refresh, start, stop };
}
