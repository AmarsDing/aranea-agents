/**
 * 新版概览页（异常驱动运维台）页面编排。
 *
 * 口径约定（与 62-overview-command-center.design.md 对齐）：
 * - 「模型调用成功率」= usage 事件行口径（success/全部行，含 cancelled 于分母）；
 * - 「Run 合格率」= runner-metrics 窗口口径（1 - error_runs/total_runs）；
 * - 两者永远带命名展示，禁止裸「成功率」字样。
 * - range 为后端 today/7d/30d/month（UTC 自然日切分），页脚显式披露。
 */
import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { useUsageStore } from '../../stores/usage';
import { usePlatformStore } from '../../stores/platform';
import { useMonitorStore } from '../../stores/monitor';
// Overview 编制计数用 list totals（limit=1 只取 total）。
import { listAgentsPaged } from '../agents/api';
import { countTeams } from '../teams/api';
import { shouldWarnZeroCost } from './pricingWarning';
import type { ModelUsageBreakdownRow, ModelUsageQuery } from './types';

const VALID_RANGES = new Set(['today', '7d', '30d', 'month']);

export function useOverviewPage() {
  const { t } = useI18n();
  const route = useRoute();
  const usageStore = useUsageStore();
  const { overview, loading, error } = storeToRefs(usageStore);

  const initialRange = String(route.query.range || 'today');
  const filters = ref<ModelUsageQuery>({
    range: VALID_RANGES.has(initialRange) ? initialRange : 'today',
    provider_code: String(route.query.provider_code || ''),
  });

  const rangeOptions = computed(() => [
    { label: t('overviewPage.rangeToday'), value: 'today' },
    { label: t('overviewPage.range7d'), value: '7d' },
    { label: t('overviewPage.range30d'), value: '30d' },
    { label: t('overviewPage.rangeMonth'), value: 'month' },
  ]);

  /** 趋势粒度跟随窗口：today→hour，其余→day（不再单独暴露筛选器）。 */
  const trendGranularity = computed<'day' | 'hour'>(() => (filters.value.range === 'today' ? 'hour' : 'day'));

  const platformStore = usePlatformStore();
  const { providerModels } = storeToRefs(platformStore);

  const providerOptions = computed(() => {
    const seen = new Map<string, string>();
    for (const m of providerModels.value) {
      const code = m.provider ?? '';
      const name = m.name ?? code;
      if (code && !seen.has(code)) seen.set(code, name);
    }
    return Array.from(seen.entries()).map(([value, label]) => ({ label, value }));
  });

  const usageUpdatedAt = ref('');

  async function loadOverview() {
    await usageStore.loadOverview({ ...filters.value }, trendGranularity.value);
    usageUpdatedAt.value = new Date().toISOString();
  }

  // —— Runner RED 组（窗口独立于 usage 窗口，卡片上显式标注） ——
  const monitorStore = useMonitorStore();
  const { runnerMetrics } = storeToRefs(monitorStore);
  const runnerWindowMinutes = ref(60);
  const runnerWindowOptions = computed(() => [
    { label: t('overviewPage.win15m'), value: 15 },
    { label: t('overviewPage.win60m'), value: 60 },
    { label: t('overviewPage.win24h'), value: 24 * 60 },
  ]);

  async function loadRunnerMetrics() {
    await monitorStore.loadRunnerMetrics(runnerWindowMinutes.value);
  }

  // —— 缓存命中率（24h 窗口；低水位阈值与告警口径一致 = 0.5） ——
  const { cacheHitStats, cacheHitDenied } = storeToRefs(usageStore);
  const CACHE_LOW_WATER = 0.5;
  const cacheHitSummary = computed(() => {
    const stats = cacheHitStats.value ?? [];
    const samples = stats.reduce((acc, s) => acc + (s.samples ?? 0), 0);
    if (!samples) return { samples: 0, p50: 0, weighted: 0, low: false };
    const weighted = stats.reduce((acc, s) => acc + s.weighted_ratio * s.samples, 0) / samples;
    const p50 = stats.reduce((acc, s) => acc + s.p50_ratio * s.samples, 0) / samples;
    return { samples, p50, weighted, low: p50 < CACHE_LOW_WATER };
  });

  // —— 模型调用组派生 ——
  const rangeSummary = computed(() => overview.value?.range ?? null);
  const yesterdaySummary = computed(() => overview.value?.yesterday ?? null);
  const monthSummary = computed(() => overview.value?.month ?? null);

  /** 环比 delta（vs 昨日同口径）；无昨日数据返回 null → 卡片显示「—」。 */
  function deltaOf(current: number | undefined, prev: number | undefined): number | null {
    const c = current ?? 0;
    const p = prev ?? 0;
    if (p <= 0) return null;
    return (c - p) / p;
  }

  const callDelta = computed(() => deltaOf(rangeSummary.value?.call_count, yesterdaySummary.value?.call_count));
  const costDelta = computed(() =>
    deltaOf(rangeSummary.value?.total_cost_micro_usd, yesterdaySummary.value?.total_cost_micro_usd),
  );

  /** 燃烧速率：range 内花费 ÷ 已过往时间（$/h）；today 口径随 range 变化，注释披露。 */
  const burnRatePerHour = computed(() => {
    const s = rangeSummary.value;
    if (!s) return null;
    const costUsd = (s.total_cost_micro_usd ?? 0) / 1_000_000;
    const now = new Date();
    const hours =
      filters.value.range === 'today'
        ? Math.max(now.getUTCHours() + now.getUTCMinutes() / 60, 0.25)
        : filters.value.range === '7d'
          ? 7 * 24
          : filters.value.range === 'month'
            ? (now.getUTCDate() - 1) * 24 + now.getUTCHours()
            : 30 * 24;
    return costUsd / Math.max(hours, 0.25);
  });

  /** 月末线性预测（仅在 range=month 时展示，月初低置信已在 UI 注明）。 */
  const monthEndForecastUsd = computed(() => {
    const s = monthSummary.value;
    if (!s) return null;
    const now = new Date();
    const dayOfMonth = now.getDate();
    if (dayOfMonth < 1) return null;
    const daysInMonth = new Date(now.getFullYear(), now.getMonth() + 1, 0).getDate();
    const spent = (s.total_cost_micro_usd ?? 0) / 1_000_000;
    return (spent / dayOfMonth) * daysInMonth;
  });

  // —— 未定价披露（tokens>0 且 cost≈0 的行；成本汇总不含这些行） ——
  const unpricedRows = computed(() =>
    (overview.value?.top_models ?? []).filter((r) =>
      shouldWarnZeroCost({ totalTokens: r.total_tokens, totalCostMicroUsd: r.total_cost_micro_usd }),
    ),
  );

  /** Top 模型成本条（Top5 + 其他）。 */
  const topModelBars = computed(() => {
    const rows = [...(overview.value?.top_models ?? [])].sort(
      (a, b) => (b.total_cost_micro_usd ?? 0) - (a.total_cost_micro_usd ?? 0),
    );
    const top = rows.slice(0, 5);
    const restCost = rows.slice(5).reduce((acc, r) => acc + (r.total_cost_micro_usd ?? 0), 0);
    const bars = top.map((r) => ({
      key: `${r.provider_code}/${r.model_api_id}`,
      label: r.model_display_name || r.model_api_id,
      costUsd: (r.total_cost_micro_usd ?? 0) / 1_000_000,
      unpriced: shouldWarnZeroCost({ totalTokens: r.total_tokens, totalCostMicroUsd: r.total_cost_micro_usd }),
    }));
    if (restCost > 0)
      bars.push({
        key: '__others',
        label: t('overviewPage.topModelsOthers'),
        costUsd: restCost / 1_000_000,
        unpriced: false,
      });
    const max = Math.max(...bars.map((b) => b.costUsd), 0.000001);
    return bars.map((b) => ({ ...b, ratio: b.costUsd / max }));
  });

  /** Agent 健康榜：按成功率升序取最差 5（调用数>0）。 */
  const worstAgents = computed<ModelUsageBreakdownRow[]>(() =>
    [...(overview.value?.top_agents ?? [])]
      .filter((r) => (r.call_count ?? 0) > 0)
      .sort((a, b) => (a.success_rate ?? 1) - (b.success_rate ?? 1))
      .slice(0, 5),
  );

  /** KPI sparkline：跟随趋势窗口最后 24 点（hour=近24h / day=近24天，卡片 caption 注明）。 */
  const callSparkline = computed(() => (overview.value?.trends ?? []).slice(-24).map((p) => p.call_count ?? 0));

  // —— 编制计数（静态配置态，仅展示区 caption；显式标注不反映运行） ——
  const inventory = ref({ agentsEnabled: 0, agentsTotal: 0, teams: 0, orgs: 0 });

  async function loadInventory() {
    const [active, total, teams] = await Promise.allSettled([
      listAgentsPaged({ status: 'active', limit: 1, offset: 0 }),
      listAgentsPaged({ limit: 1, offset: 0 }),
      countTeams(),
      platformStore.loadTaxonomyTree('organization'),
    ]);
    inventory.value = {
      agentsEnabled: active.status === 'fulfilled' ? active.value.total : 0,
      agentsTotal: total.status === 'fulfilled' ? total.value.total : 0,
      teams: teams.status === 'fulfilled' ? teams.value : 0,
      orgs: platformStore.taxonomyTree.length,
    };
  }

  /** 预算利用率（quota_dashboard；未配置预算时 UI 显示灰 hint）。 */
  const quota = computed(() => overview.value?.quota_dashboard ?? null);

  async function refreshAll() {
    await Promise.allSettled([loadOverview(), loadRunnerMetrics(), usageStore.loadCacheHitStats(24), loadInventory()]);
  }

  function setRange(range: string) {
    filters.value = { ...filters.value, range };
    void loadOverview();
  }

  function setProvider(provider: string) {
    filters.value = { ...filters.value, provider_code: provider };
    void loadOverview();
  }

  onMounted(() => {
    void refreshAll();
    void platformStore.loadProviderModels();
  });

  return {
    t,
    overview,
    loading,
    error,
    filters,
    rangeOptions,
    providerOptions,
    setRange,
    setProvider,
    trendGranularity,
    rangeSummary,
    yesterdaySummary,
    callDelta,
    costDelta,
    burnRatePerHour,
    monthEndForecastUsd,
    unpricedRows,
    topModelBars,
    worstAgents,
    callSparkline,
    quota,
    runnerMetrics,
    runnerWindowMinutes,
    runnerWindowOptions,
    loadRunnerMetrics,
    cacheHitSummary,
    cacheHitDenied,
    inventory,
    usageUpdatedAt,
    refreshAll,
  };
}
