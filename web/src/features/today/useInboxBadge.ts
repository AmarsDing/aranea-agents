/**
 * 侧栏「工作台」入口的待办角标：读 /v1/inbox/summary，与工作台列表同一口径。
 * 读取失败时保留上一次的数，不清零也不报错（角标只是提示）。
 */
import { onMounted, onUnmounted, ref } from 'vue';
import { getInboxSummary } from '../approval/api';

const POLL_INTERVAL_MS = 60_000;

// 模块级共享：工作台列表每次读到新总数就直接写进来，角标不用等下一轮轮询。
const count = ref(0);

export function setInboxBadge(total: number): void {
  count.value = Math.max(0, total);
}

export function useInboxBadge() {
  let timer: ReturnType<typeof setInterval> | null = null;

  async function refresh(): Promise<void> {
    try {
      count.value = (await getInboxSummary()).total;
    } catch (err) {
      console.warn('[today] inbox badge refresh failed', err);
    }
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

  return { count, refresh };
}
