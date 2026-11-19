/**
 * 作战总台 LLM 成本卡（E阶段 W2b D6 V5 后半 / F2，验收 #8 对账）。
 *
 * 数据源：usage `GetUsageOverview` 一次返回 today/month 汇总 + top_agents +
 * quota_dashboard，与用量概览页同 RPC 同口径——验收 #8 对账即
 * 「卡片今日/本月成本 == usage 页同口径数字」。
 *
 * 诚实口径（与 cockpit 家族一致）：
 * - 未加载（loaded=false）一律显 —，禁把未加载当 0；
 * - 刷新失败保留上次成功数据 + 挂错误注记，不伪造清零。
 *
 * 挂载状态（2026-11-10）：WorkbenchCockpitPage.vue 为 peer 在制品（YOU 签批改造），
 * 挂载一行待 peer 收口后 follow-up（W4 登记）；本组合式与组件已可独立测试/复用。
 */
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { getModelUsageOverview } from '../usage/api';
import type { UsageQuotaDashboard } from '../usage/types';

export type LlmCostBudgetState = 'none' | 'ok' | 'warn' | 'over';

/** 预算告警线：利用率 ≥0.8 预警、≥1 超支（拍板值，quota 无语义字段时以此为唯一裁决）。 */
export const BUDGET_WARN_RATIO = 0.8;

/** 预算告警态：未配置配额（configured_count=0/无 dashboard）→ none（不显示，不假装安全）。 */
export function deriveBudgetState(q?: UsageQuotaDashboard | null): LlmCostBudgetState {
  if (!q || q.configured_count <= 0) return 'none';
  if (q.max_utilization_ratio >= 1) return 'over';
  if (q.max_utilization_ratio >= BUDGET_WARN_RATIO) return 'warn';
  return 'ok';
}

export interface LlmCostTopAgent {
  agentKey: string;
  costMicroUsd: number;
}

/** 顶部 agent 条数（卡面高度约束，拍板 3 条）。 */
export const TOP_AGENTS_LIMIT = 3;

export function useLlmCostCard(opts: { pollMs?: number; autoStart?: boolean } = {}) {
  const pollMs = opts.pollMs ?? 60_000;

  const loading = ref(false);
  /** 至少成功过一次：false → 卡面显 —（诚实口径） */
  const loaded = ref(false);
  /** 最近一次刷新失败原因（成功即清空；失败时保留旧数据展示） */
  const error = ref('');
  const todayCostMicroUsd = ref(0);
  const monthCostMicroUsd = ref(0);
  const topAgents = ref<LlmCostTopAgent[]>([]);
  const quota = ref<UsageQuotaDashboard | null>(null);
  const lastUpdatedAt = ref('');

  const budgetState = computed(() => deriveBudgetState(quota.value));

  let timer: ReturnType<typeof setInterval> | null = null;

  async function refresh(): Promise<void> {
    if (loading.value) return; // 防重入（手动刷新与轮询撞车时只跑一份）
    loading.value = true;
    try {
      const o = await getModelUsageOverview({});
      todayCostMicroUsd.value = o.today.total_cost_micro_usd;
      monthCostMicroUsd.value = o.month.total_cost_micro_usd;
      topAgents.value = (o.top_agents ?? [])
        .filter((r) => r.agent_key !== '')
        .slice(0, TOP_AGENTS_LIMIT)
        .map((r) => ({ agentKey: r.agent_key, costMicroUsd: r.total_cost_micro_usd }));
      quota.value = o.quota_dashboard ?? null;
      error.value = '';
      loaded.value = true;
      lastUpdatedAt.value = new Date().toISOString();
    } catch (e) {
      // 诚实口径：保留上次成功数据，只挂错误注记（不清零假数据）
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      loading.value = false;
    }
  }

  function start(): void {
    if (!timer) timer = setInterval(() => void refresh(), pollMs);
  }

  function stop(): void {
    if (timer) {
      clearInterval(timer);
      timer = null;
    }
  }

  onMounted(() => {
    if (opts.autoStart === false) return; // 测试锚：手动驱动
    void refresh();
    start();
  });
  onUnmounted(stop);

  return {
    loading,
    loaded,
    error,
    todayCostMicroUsd,
    monthCostMicroUsd,
    topAgents,
    quota,
    budgetState,
    lastUpdatedAt,
    refresh,
    start,
    stop,
  };
}
