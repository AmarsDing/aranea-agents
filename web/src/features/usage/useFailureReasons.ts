/**
 * 失败原因汇总（运行与治理审查 P1，2026-11-19）：把失败的模型调用按错误归并——次数、涉及的 Agent、最近一次；
 * 认得出的错误给出原因和修复去处。用量成本页和健康与告警页共用。
 * 数据：GET /v1/usage/events，按失败类状态分别取（每种最多 200 条），与页面同一组筛选。
 */
import { ref } from 'vue';
import { listModelUsageEvents } from './api';
import type { ModelTokenUsageEvent, ModelUsageQuery } from './types';

// 后端把 error / fail 归一成 failed（biz/usage NormalizeStatus），再按 error 查会把同一批失败数两遍
const FAILED_STATUSES = ['failed', 'timeout'] as const;

/** 认得出的错误：按错误码 / 错误信息里的关键词匹配（不分大小写），cause 是 ops.failures.cause 下的文案键；中文关键词（余额不足、超时）写成转义，免得被当界面文字 */
const KNOWN_CAUSES: Array<{ match: RegExp; cause: string; fix: 'agent' | 'credentials' | '' }> = [
  { match: /LLM_PROVIDER_MODEL\/NOT_FOUND|model not found/i, cause: 'modelMissing', fix: 'agent' },
  { match: /UNAUTHENTICATED|invalid api key|incorrect api key|\b401\b/i, cause: 'keyInvalid', fix: 'credentials' },
  { match: /insufficient_quota|quota|\u4f59\u989d\u4e0d\u8db3|\b402\b/i, cause: 'quota', fix: 'credentials' },
  { match: /rate limit|too many requests|\b429\b/i, cause: 'rateLimited', fix: '' },
  { match: /timeout|deadline exceeded|\u8d85\u65f6/i, cause: 'timeout', fix: '' },
];

export type FailureReason = {
  key: string;
  message: string;
  /** 认得出时为 ops.failures.cause 下的键，否则空 */
  cause: string;
  fix: 'agent' | 'credentials' | '';
  count: number;
  agents: Array<{ id: string; name: string }>;
  lastAt: string;
};

/** 归并用的错误键：去掉 ID、长数字这类每次都不同的部分 */
export function failureKey(e: Pick<ModelTokenUsageEvent, 'error_code' | 'error_message'>): string {
  const raw = `${e.error_code ?? ''} ${e.error_message ?? ''}`.trim() || 'unknown';
  return raw
    .replace(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi, '…')
    .replace(/\d{4,}/g, '…')
    .slice(0, 200);
}

/** 纯函数：事件 → 按错误归并的原因列表（次数从多到少） */
export function groupFailures(events: ModelTokenUsageEvent[]): FailureReason[] {
  const groups = new Map<string, FailureReason>();
  for (const e of events) {
    const key = failureKey(e);
    let g = groups.get(key);
    if (!g) {
      const text = `${e.error_code ?? ''} ${e.error_message ?? ''}`;
      const known = KNOWN_CAUSES.find((k) => k.match.test(text));
      g = {
        key,
        message: (e.error_message || e.error_code || '').trim(),
        cause: known?.cause ?? '',
        fix: known?.fix ?? '',
        count: 0,
        agents: [],
        lastAt: e.occurred_at,
      };
      groups.set(key, g);
    }
    g.count += 1;
    if (e.occurred_at > g.lastAt) g.lastAt = e.occurred_at;
    const id = e.agent_id || e.agent_key || '';
    if (id && !g.agents.some((a) => a.id === id)) g.agents.push({ id, name: e.agent_name || e.agent_key || id });
  }
  return [...groups.values()].sort((a, b) => b.count - a.count || b.lastAt.localeCompare(a.lastAt));
}

export function useFailureReasons() {
  const reasons = ref<FailureReason[]>([]);
  const total = ref(0);
  const error = ref('');
  const loading = ref(false);

  async function load(query: Omit<ModelUsageQuery, 'status' | 'limit' | 'offset'>) {
    loading.value = true;
    try {
      const results = await Promise.all(
        FAILED_STATUSES.map((status) => listModelUsageEvents({ ...query, status, limit: 200 })),
      );
      total.value = results.reduce((sum, r) => sum + r.total, 0);
      const unique = new Map(results.flatMap((r) => r.items).map((e) => [e.id, e]));
      reasons.value = groupFailures([...unique.values()]);
      error.value = '';
    } catch (err) {
      error.value = err instanceof Error ? err.message : String(err);
    } finally {
      loading.value = false;
    }
  }

  return { reasons, total, error, loading, load };
}
