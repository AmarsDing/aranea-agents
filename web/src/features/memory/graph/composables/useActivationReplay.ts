// Service: 扩散激活回放 — 调 SpreadingActivation API，按 hop_count 分组，定时器逐跳点亮节点/边。
import { computed, ref, readonly } from 'vue';
import type { SpreadingActivationResult, ActivationPathStep } from '../../types';
import { useMemoryStore } from '../../../../stores/memory';

export const HOP_INTERVAL_MS = 600;
const REPLAY_HOPS = 3;
const REPLAY_TOP_K = 20;

/** 扩散激活回放状态机：replay → playing → done / stop。 */
export function useActivationReplay() {
  const memoryStore = useMemoryStore();
  const playing = ref(false);
  const error = ref('');
  const centerId = ref('');
  const results = ref<SpreadingActivationResult[]>([]);
  const activeHops = ref<Set<number>>(new Set());

  let timer: ReturnType<typeof setTimeout> | null = null;
  let maxHop = 0;

  /** 节点 → activation 值映射（仅包含已点亮的节点）。 */
  const activationMap = computed(() => {
    const map = new Map<string, number>();
    for (const r of results.value) {
      if (activeHops.value.has(r.hop_count)) {
        map.set(r.node_id, r.activation);
      }
    }
    return map;
  });

  /** Top-K 激活排行（按 activation 降序，全量返回不受点亮进度影响）。 */
  const topKRanking = computed(() => [...results.value].sort((a, b) => b.activation - a.activation));

  /** 所有 activation_path 经过的边 key 集合（仅包含已点亮跳的路径）。 */
  const highlightEdges = computed(() => {
    const set = new Set<string>();
    for (const r of results.value) {
      if (!activeHops.value.has(r.hop_count)) continue;
      for (const step of r.activation_path) {
        set.add(edgeKey(step));
      }
    }
    return set;
  });

  function edgeKey(step: ActivationPathStep): string {
    return `${step.from_node_id}->${step.to_node_id}:${step.relation_type}`;
  }

  /** 查询节点 activation 值（未点亮时返回 0）。 */
  function activationOf(nodeId: string): number {
    return activationMap.value.get(nodeId) ?? 0;
  }

  function clearTimer() {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
  }

  function scheduleNextHop() {
    const nextHop = Math.max(...activeHops.value) + 1;
    if (nextHop > maxHop) {
      playing.value = false;
      return;
    }
    timer = setTimeout(() => {
      activeHops.value = new Set([...activeHops.value, nextHop]);
      if (nextHop >= maxHop) {
        playing.value = false;
      } else {
        scheduleNextHop();
      }
    }, HOP_INTERVAL_MS);
  }

  /** 开始回放：调 API → 按 hop 分组 → 定时器逐跳点亮。 */
  async function replay(id: string) {
    stop();
    error.value = '';
    centerId.value = id;

    try {
      const res = await memoryStore.fetchSpreadingActivation(id, { hops: REPLAY_HOPS, top_k: REPLAY_TOP_K });
      results.value = res.items;
      maxHop = res.items.reduce((max, r) => Math.max(max, r.hop_count), 0);

      // hop 0 立即激活
      activeHops.value = new Set([0]);
      playing.value = true;

      if (maxHop > 0) {
        scheduleNextHop();
      } else {
        playing.value = false;
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      playing.value = false;
    }
  }

  /** 停止回放：清除定时器，复位所有状态。 */
  function stop() {
    clearTimer();
    playing.value = false;
    activeHops.value = new Set();
    results.value = [];
    centerId.value = '';
  }

  /** 组件 unmount 时调用，确保定时器清理。 */
  function dispose() {
    stop();
  }

  return {
    playing: readonly(playing),
    error: readonly(error),
    centerId: readonly(centerId),
    activeHops: readonly(activeHops),
    topKRanking,
    highlightEdges,
    activationOf,
    replay,
    stop,
    dispose,
  };
}
