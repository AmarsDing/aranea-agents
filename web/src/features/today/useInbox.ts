/**
 * 工作台收件箱：读 /v1/inbox，写 /v1/inbox/decide。
 * 排序、权限过滤、去重都在服务端，这里保持服务端给出的顺序。
 */
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { decideInboxItem, isConflictError, listInbox, type InboxAction, type InboxItemRow } from '../approval/api';
import { useNavStore } from '../../stores/nav';
import { parseKratosApiError } from '../../utils/kratosError';
import { setInboxBadge } from './useInboxBadge';

const POLL_INTERVAL_MS = 30_000;
const PAGE_SIZE = 200;
/** 决议成功后隐藏该条的时长（来源侧落库可能有秒级延迟）。 */
const SETTLED_TTL_MS = 120_000;

export type DecideOutcome = { ok: true } | { ok: false; gone: boolean; message: string };

export function useInbox() {
  const nav = useNavStore();
  const fetched = ref<InboxItemRow[]>([]);
  const serverTotal = ref(0);
  const degraded = ref<string[]>([]);
  const loading = ref(false);
  /** 首次读取完成前为 false（区分「加载中」与「确实为空」） */
  const loaded = ref(false);
  /** 最近一次读取整体失败（保留旧数据，不清空） */
  const failed = ref(false);
  const updatedAt = ref('');
  const busyIds = ref<Set<string>>(new Set());
  const settled = ref<Map<string, number>>(new Map());

  let timer: ReturnType<typeof setInterval> | null = null;
  let inflight: Promise<void> | null = null;

  const rows = computed(() => fetched.value.filter((row) => !settled.value.has(row.id)));
  const hiddenCount = computed(() => fetched.value.length - rows.value.length);
  const total = computed(() => Math.max(rows.value.length, serverTotal.value - hiddenCount.value));

  function pruneSettled() {
    const now = Date.now();
    const next = new Map([...settled.value].filter(([, until]) => until > now));
    if (next.size !== settled.value.size) settled.value = next;
  }

  async function refresh(): Promise<void> {
    if (inflight) return inflight;
    loading.value = true;
    inflight = (async () => {
      try {
        const res = await listInbox({ pageSize: PAGE_SIZE });
        pruneSettled();
        fetched.value = res.items;
        serverTotal.value = res.total;
        setInboxBadge(res.total);
        degraded.value = res.degraded;
        failed.value = false;
        updatedAt.value = new Date().toISOString();
      } catch (err) {
        console.warn('[today] inbox refresh failed', err);
        failed.value = true;
      } finally {
        loading.value = false;
        loaded.value = true;
        inflight = null;
      }
    })();
    return inflight;
  }

  function setBusy(id: string, on: boolean) {
    const next = new Set(busyIds.value);
    if (on) next.add(id);
    else next.delete(id);
    busyIds.value = next;
  }

  /** 统一决议。条目已被别人处理（409）时刷新列表并返回 gone。 */
  async function decide(row: InboxItemRow, action: InboxAction, note = ''): Promise<DecideOutcome> {
    if (busyIds.value.has(row.id)) return { ok: false, gone: false, message: '' };
    setBusy(row.id, true);
    try {
      await decideInboxItem(row.id, action, note);
      settled.value = new Map(settled.value).set(row.id, Date.now() + SETTLED_TTL_MS);
      void refresh();
      return { ok: true };
    } catch (err) {
      if (isConflictError(err)) {
        await refresh();
        return { ok: false, gone: true, message: '' };
      }
      return { ok: false, gone: false, message: parseKratosApiError(err).message };
    } finally {
      setBusy(row.id, false);
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

  // 权限异步下发后重取：服务端按新权限过滤可见来源。
  watch(
    () => nav.permissions.join(','),
    () => void refresh(),
  );

  onUnmounted(() => {
    if (timer) clearInterval(timer);
    document.removeEventListener('visibilitychange', onVisibility);
  });

  return { rows, total, degraded, loading, loaded, failed, updatedAt, busyIds, refresh, decide };
}
