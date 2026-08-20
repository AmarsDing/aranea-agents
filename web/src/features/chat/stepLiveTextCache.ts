/**
 * stepLiveTextCache — running step 流式文本的会话级缓存（P3-resume 2026-08-20）。
 *
 * 后端 checkpoint（step.updated 全量快照，≤1s 节流落库）之外的本 tab 兜底：
 * delta append 时把累积文本节流（500ms）写 sessionStorage；刷新后 hydrate 到
 * 非终态 step 时按「同一 delta 流 ⇒ 前缀一致，取长者」合并，把可见文本空洞
 * 压缩到最后一次 flush 以内。终态 step 到达（step.completed 等，critical 事件
 * 必达）时清除。
 *
 * 与 channelWsCursor 同哲学：内存 map 优先，sessionStorage 跨刷新存活（同 tab）。
 * stepID = turnID + "-s" + n 全局唯一，孤儿条目永不误合并；读取时惰性 TTL 清理。
 */

export type StepLiveText = {
  content: string;
  reasoning: string;
  updatedAt: number;
};

const STORAGE_PREFIX = 'aranea:step-live:';
const FLUSH_INTERVAL_MS = 500;
const ORPHAN_TTL_MS = 2 * 60 * 60 * 1000; // 2h：崩溃/直接关 tab 留下的孤儿条目惰性清理
const MAX_ENTRIES = 100; // 硬上限兜底；终态清理是主路径

const TERMINAL_STATUSES = new Set(['completed', 'failed', 'cancelled', 'interrupted', 'skipped']);

const liveById = new Map<string, StepLiveText>();
const flushTimers = new Map<string, ReturnType<typeof setTimeout>>();

function storageKey(stepId: string): string {
  return `${STORAGE_PREFIX}${stepId.trim()}`;
}

function isStorageAvailable(): boolean {
  return typeof sessionStorage !== 'undefined';
}

function writeToStorage(stepId: string, value: StepLiveText): void {
  if (!isStorageAvailable()) return;
  try {
    sessionStorage.setItem(storageKey(stepId), JSON.stringify(value));
    evictIfNeeded();
  } catch {
    /* private mode / quota — 缓存是 best-effort，静默降级 */
  }
}

function readFromStorage(stepId: string): StepLiveText | undefined {
  if (!isStorageAvailable()) return undefined;
  try {
    const raw = sessionStorage.getItem(storageKey(stepId));
    if (!raw) return undefined;
    const parsed = JSON.parse(raw) as StepLiveText;
    if (typeof parsed?.content !== 'string' || typeof parsed?.reasoning !== 'string') return undefined;
    return parsed;
  } catch {
    return undefined;
  }
}

function removeFromStorage(stepId: string): void {
  if (!isStorageAvailable()) return;
  try {
    sessionStorage.removeItem(storageKey(stepId));
  } catch {
    /* ignore */
  }
}

/** 超硬上限时按 updatedAt 淘汰最老一半（仅 flush 路径触发，频率 ≤1/500ms）。 */
function evictIfNeeded(): void {
  if (!isStorageAvailable() || liveById.size <= MAX_ENTRIES) return;
  const sorted = [...liveById.entries()].sort((a, b) => a[1].updatedAt - b[1].updatedAt);
  const evictCount = Math.floor(sorted.length / 2);
  for (let i = 0; i < evictCount; i++) {
    const [id] = sorted[i];
    liveById.delete(id);
    removeFromStorage(id);
  }
}

export function isTerminalStepStatus(status: string | undefined): boolean {
  return TERMINAL_STATUSES.has(String(status ?? '').toLowerCase());
}

/**
 * delta append 后记录累积值（非 chunk）。内存即时更新，sessionStorage 节流 flush。
 */
export function noteStepLiveText(stepId: string, field: 'content' | 'reasoning', accumulated: string): void {
  const id = stepId.trim();
  if (!id) return;
  const entry = liveById.get(id) ?? { content: '', reasoning: '', updatedAt: 0 };
  entry[field] = accumulated;
  entry.updatedAt = Date.now();
  liveById.set(id, entry);
  if (flushTimers.has(id)) return;
  flushTimers.set(
    id,
    setTimeout(() => {
      flushTimers.delete(id);
      const cur = liveById.get(id);
      if (cur) writeToStorage(id, cur);
    }, FLUSH_INTERVAL_MS),
  );
}

/** 内存优先，sessionStorage 兜底（跨刷新）。孤儿（超 TTL）惰性清除。 */
export function readStepLiveText(stepId: string): StepLiveText | undefined {
  const id = stepId.trim();
  if (!id) return undefined;
  const cached = liveById.get(id) ?? readFromStorage(id);
  if (!cached) return undefined;
  if (Date.now() - cached.updatedAt > ORPHAN_TTL_MS) {
    clearStepLiveText(id);
    return undefined;
  }
  liveById.set(id, cached);
  return cached;
}

/** 终态到达时调用：清内存 + 待写 timer + storage。 */
export function clearStepLiveText(stepId: string): void {
  const id = stepId.trim();
  if (!id) return;
  liveById.delete(id);
  const timer = flushTimers.get(id);
  if (timer) {
    clearTimeout(timer);
    flushTimers.delete(id);
  }
  removeFromStorage(id);
}

/** 全量清理（store clearAll / 测试隔离）。 */
export function clearAllStepLiveText(): void {
  liveById.clear();
  for (const timer of flushTimers.values()) clearTimeout(timer);
  flushTimers.clear();
  if (!isStorageAvailable()) return;
  try {
    const keys: string[] = [];
    for (let i = 0; i < sessionStorage.length; i++) {
      const k = sessionStorage.key(i);
      if (k?.startsWith(STORAGE_PREFIX)) keys.push(k);
    }
    for (const k of keys) sessionStorage.removeItem(k);
  } catch {
    /* ignore */
  }
}

/**
 * hydrate/upsert 时合并缓存：仅对非终态 step 生效。缓存与 DB/快照同源（同一
 * delta 流），前缀一致时取长者；前缀分叉（理论上不可能，防御分支）信服务端。
 * 返回原对象或合并后的新对象。
 */
export function mergeStepLiveText<T extends { ID: string; Content: string; Reasoning: string; Status: string }>(
  step: T,
): T {
  if (isTerminalStepStatus(step.Status)) return step;
  const cached = readStepLiveText(step.ID);
  if (!cached) return step;
  let merged: T | undefined;
  if (cached.content.length > (step.Content?.length ?? 0) && cached.content.startsWith(step.Content ?? '')) {
    merged = { ...(merged ?? step), Content: cached.content };
  }
  if (cached.reasoning.length > (step.Reasoning?.length ?? 0) && cached.reasoning.startsWith(step.Reasoning ?? '')) {
    merged = { ...(merged ?? step), Reasoning: cached.reasoning };
  }
  return merged ?? step;
}
