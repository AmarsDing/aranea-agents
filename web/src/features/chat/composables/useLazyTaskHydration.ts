/**
 * useLazyTaskHydration — 历史 task 折叠卡的懒水合编排（2026-07-23 设计 §4.3）。
 *
 * - IntersectionObserver（root=消息滚动容器, threshold=0.4）：折叠卡进入视口
 *   启动 500ms dwell 定时器 → hydrate；离开视口取消（快速滑过不触发）。
 * - expandTask(taskId)：点击卡片立即水合。
 * - 滚动锚定：卡片原位置在视口上方时，水合渲染后按 scrollHeight 增量补偿
 *   scrollTop，视口不跳动。
 * - collapsedIds：水合后手动「收起执行过程」的 UI 态；数据保留 store，
 *   再展开零请求（设计 P6）。
 *
 * 网络请求不在这里——hydrate 由调用方注入（store action），本 composable
 * 只做 DOM 编排，便于单测 mock。
 */
import { nextTick, onBeforeUnmount, ref, type Ref } from 'vue';

export const HYDRATE_DWELL_MS = 500;
const IO_THRESHOLD = 0.4;

/** ChatMessageList provide 的滚动容器 inject key。 */
export const CHAT_SCROLL_EL_KEY = 'chat-messages-scroll-el';

export type LazyTaskHydrationOpts = {
  /** 消息滚动容器。 */
  scrollEl: Ref<HTMLElement | null>;
  /** 折叠卡判定：true = 该 task 未水合且未在水合中，需要自动水合。 */
  needsHydration: (taskId: string) => boolean;
  /** 水合触发（store action，幂等）。 */
  hydrate: (taskId: string) => Promise<void>;
};

export function useLazyTaskHydration(opts: LazyTaskHydrationOpts) {
  const collapsedIds = ref(new Set<string>());
  let observer: IntersectionObserver | null = null;
  const dwellTimers = new Map<string, ReturnType<typeof setTimeout>>();
  const observed = new Map<string, Element>();

  function cancelDwell(taskId: string) {
    const timer = dwellTimers.get(taskId);
    if (timer) {
      clearTimeout(timer);
      dwellTimers.delete(taskId);
    }
  }

  function handleEntries(entries: IntersectionObserverEntry[]) {
    for (const e of entries) {
      const el = e.target as HTMLElement;
      const taskId = el.dataset.taskId;
      if (!taskId) continue;
      if (e.isIntersecting) {
        if (!opts.needsHydration(taskId) || dwellTimers.has(taskId)) continue;
        dwellTimers.set(
          taskId,
          setTimeout(() => {
            dwellTimers.delete(taskId);
            void hydrateWithAnchor(taskId);
          }, HYDRATE_DWELL_MS),
        );
      } else {
        cancelDwell(taskId);
      }
    }
  }

  /** 同步观察集合：tasks 渲染/水合状态变化后由调用方 nextTick 触发。 */
  function syncCards() {
    const root = opts.scrollEl.value;
    if (!root) return;
    if (!observer && typeof IntersectionObserver !== 'undefined') {
      observer = new IntersectionObserver(handleEntries, { root, threshold: IO_THRESHOLD });
    }
    const seen = new Set<string>();
    for (const el of root.querySelectorAll<HTMLElement>('.task-card[data-task-id]')) {
      const taskId = el.dataset.taskId;
      if (!taskId) continue;
      seen.add(taskId);
      const watching = observed.has(taskId);
      if (opts.needsHydration(taskId)) {
        if (!watching) {
          observer?.observe(el);
          observed.set(taskId, el);
        }
      } else if (watching) {
        observer?.unobserve(observed.get(taskId)!);
        observed.delete(taskId);
        cancelDwell(taskId);
      }
    }
    // 清理已从 DOM 移除的卡片（会话切换等）。
    for (const [taskId, el] of [...observed]) {
      if (!seen.has(taskId)) {
        observer?.unobserve(el);
        observed.delete(taskId);
        cancelDwell(taskId);
      }
    }
  }

  /** 滚动锚定：仅当卡片原位置在视口上方时，按高度增量补偿 scrollTop。 */
  async function hydrateWithAnchor(taskId: string) {
    const el = opts.scrollEl.value;
    const card = el?.querySelector<HTMLElement>(`.task-card[data-task-id="${CSS.escape(taskId)}"]`);
    const wasAbove =
      !!el && !!card && card.getBoundingClientRect().top < el.getBoundingClientRect().top;
    const prevHeight = el?.scrollHeight ?? 0;
    await opts.hydrate(taskId);
    await nextTick();
    if (el && wasAbove) {
      el.scrollTop += el.scrollHeight - prevHeight;
    }
  }

  /** 点击卡片立即水合（无 dwell）。 */
  function expandTask(taskId: string) {
    void hydrateWithAnchor(taskId);
  }

  function isCollapsed(taskId: string): boolean {
    return collapsedIds.value.has(taskId);
  }

  function toggleCollapse(taskId: string) {
    if (collapsedIds.value.has(taskId)) collapsedIds.value.delete(taskId);
    else collapsedIds.value.add(taskId);
  }

  onBeforeUnmount(() => {
    observer?.disconnect();
    for (const t of dwellTimers.values()) clearTimeout(t);
    dwellTimers.clear();
    observed.clear();
  });

  return { collapsedIds, isCollapsed, toggleCollapse, expandTask, syncCards };
}
