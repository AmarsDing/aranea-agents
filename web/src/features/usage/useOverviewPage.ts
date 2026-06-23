import { computed, onMounted, reactive, ref } from 'vue';
import { useRoute } from 'vue-router';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { useUsageStore } from '../../stores/usage';
import { usePlatformStore } from '../../stores/platform';
import { useMonitorStore } from '../../stores/monitor';
// TECH-DEBT(O-8): 概览页直接调 agents/teams API 取 total，未走 Store。
//   原因：Store 的 load 逻辑含分页/过滤，概览只需 total；迁入 Store 需重构 load 签名。
//   待办：agent/team Store 增加 countOnly 模式后迁入。
import { listAgentsPaged } from '../agents/api';
import type { AgentListQuery } from '../agents/types';
import type { ModelUsageQuery } from './types';
import { formatUsdFromMicro, formatCount as fmtCount, formatPercent as fmtPercent } from './moneyFormat';
import { useMonitorRunNavigation } from '../monitor/useMonitorRunNavigation';
// TECH-DEBT(O-9): listTeams 拉取全量数据仅取 length，Team 数量多时有性能问题。
//   待办：后端 ListTeams 支持分页后改用 total 字段，或新增 CountTeams RPC。
import { listTeams } from '../teams/api';

const VALID_RANGES = new Set(['today', '7d', '30d', 'month']);

export function useOverviewPage() {
  const { t } = useI18n();
  const route = useRoute();
  const usageStore = useUsageStore();
  const { overview, loading, error } = storeToRefs(usageStore);
  const trendGranularity = ref<'day' | 'hour'>('day');
  const initialRange = String(route.query.range || '30d');
  const filters = reactive<ModelUsageQuery>({
    range: VALID_RANGES.has(initialRange) ? initialRange : '30d',
    provider_code: '',
    model_api_id: '',
    status: '',
  });

  const rangeOptions = computed(() => [
    { label: t('overviewPage.rangeToday'), value: 'today' },
    { label: t('overviewPage.range7d'), value: '7d' },
    { label: t('overviewPage.range30d'), value: '30d' },
    { label: t('overviewPage.rangeMonth'), value: 'month' },
  ]);

  const statusOptions = computed(() => [
    { label: t('overviewPage.statusSuccess'), value: 'success' },
    { label: t('overviewPage.statusFailed'), value: 'failed' },
    { label: t('overviewPage.statusAbnormal'), value: 'error' },
    { label: t('overviewPage.statusCancelled'), value: 'cancelled' },
    { label: t('overviewPage.statusTimeout'), value: 'timeout' },
  ]);

  const granularityOptions = computed(() => [
    { label: t('overviewPage.granularityDay'), value: 'day' },
    { label: t('overviewPage.granularityHour'), value: 'hour' },
  ]);

  const platformStore = usePlatformStore();
  const { providerModels } = storeToRefs(platformStore);

  const providerOptions = computed(() => {
    const seen = new Map<string, string>();
    for (const m of providerModels.value) {
      const code = m.provider ?? '';
      const name = m.name ?? code;
      if (code && !seen.has(code)) {
        seen.set(code, name);
      }
    }
    return Array.from(seen.entries()).map(([value, label]) => ({ label, value }));
  });

  const modelOptions = computed(() => {
    const provider = filters.provider_code;
    const seen = new Map<string, string>();
    for (const m of providerModels.value) {
      if (provider && m.provider !== provider) continue;
      const apiId = m.model ?? '';
      const name = m.name ?? apiId;
      if (apiId && !seen.has(apiId)) {
        seen.set(apiId, name);
      }
    }
    return Array.from(seen.entries()).map(([value, label]) => ({ label, value }));
  });

  function onProviderChange() {
    const currentModel = filters.model_api_id;
    if (currentModel) {
      const valid = modelOptions.value.some((o) => o.value === currentModel);
      if (!valid) filters.model_api_id = '';
    }
    loadOverview();
  }

  const providerHealthLoading = ref(true);

  async function loadProviderHealth() {
    providerHealthLoading.value = true;
    try {
      await platformStore.loadProviderModels();
    } catch {
      // silent
    } finally {
      providerHealthLoading.value = false;
    }
  }

  async function loadOverview() {
    await usageStore.loadOverview({ ...filters }, trendGranularity.value);
  }

  const monitorStore = useMonitorStore();
  const { runnerMetrics, runnerLoading } = storeToRefs(monitorStore);
  const runnerWindowMinutes = ref(60);
  const { openRunsTab } = useMonitorRunNavigation();

  async function reloadRunnerMetrics() {
    await monitorStore.loadRunnerMetrics(runnerWindowMinutes.value);
  }

  function formatCount(value?: number) {
    return fmtCount(value);
  }

  function formatMoney(value?: number) {
    return formatUsdFromMicro(value);
  }

  function formatPercent(value?: number) {
    return fmtPercent(value);
  }

  const agentStats = ref({ active: 0, total: 0 });

  async function loadAgentStats() {
    try {
      const [activeResult, totalResult] = await Promise.all([
        listAgentsPaged({ status: 'active', limit: 1, offset: 0 } as AgentListQuery),
        listAgentsPaged({ limit: 1, offset: 0 } as AgentListQuery),
      ]);
      agentStats.value = {
        active: activeResult.total,
        total: totalResult.total,
      };
    } catch {
      // silent
    }
  }

  const providerCount = computed(() => {
    const seen = new Set<string>();
    for (const m of providerModels.value) {
      const code = m.provider ?? '';
      if (code) seen.add(code);
    }
    return seen.size;
  });

  const categoryCount = ref(0);
  const teamCount = ref(0);

  async function loadCategoryCount() {
    try {
      await platformStore.loadTaxonomyTree('organization');
      categoryCount.value = platformStore.taxonomyTree.length;
    } catch {
      // silent
    }
  }

  async function loadTeamCount() {
    try {
      const teams = await listTeams();
      teamCount.value = teams.length;
    } catch {
      // silent
    }
  }

  const tokenTrendDialogOpen = ref(false);

  function openTokenTrendDialog() {
    tokenTrendDialogOpen.value = true;
  }

  const username = computed(() => {
    try {
      const raw = localStorage.getItem('auth_user');
      if (raw) {
        const parsed = JSON.parse(raw);
        if (parsed.username) return parsed.username;
      }
    } catch {
      // silent
    }
    return 'Admin';
  });

  const providerHealthSummary = computed(() => {
    const models = providerModels.value;
    const active = models.filter((m: { status: string }) => m.status === 'active' || !m.status).length;
    const degraded = models.filter((m: { status: string }) => m.status === 'degraded').length;
    return { active, degraded, total: models.length };
  });

  const sessionActiveCount = computed(() => overview.value?.today?.call_count ?? 0);

  const sessionSparkline = computed(() => {
    const trends = overview.value?.trends ?? [];
    return trends.slice(-24).map((t: { call_count: number }) => t.call_count ?? 0);
  });

  const runnerStats = computed(() => ({
    totalRuns: runnerMetrics.value?.total_runs ?? 0,
    errorRuns: runnerMetrics.value?.error_runs ?? 0,
    successRate: (runnerMetrics.value?.success_rate ?? 0) * 100,
    errorRate: (runnerMetrics.value?.error_rate ?? 0) * 100,
  }));

  onMounted(() => {
    void loadProviderHealth();
    void reloadRunnerMetrics();
    void loadAgentStats();
    void loadCategoryCount();
    void loadTeamCount();
  });

  return {
    t,
    overview,
    loading,
    error,
    trendGranularity,
    filters,
    rangeOptions,
    statusOptions,
    granularityOptions,
    providerOptions,
    modelOptions,
    onProviderChange,
    loadOverview,
    formatCount,
    formatMoney,
    formatPercent,
    providerModels,
    providerHealthLoading,
    runnerMetrics,
    runnerLoading,
    runnerWindowMinutes,
    reloadRunnerMetrics,
    openRunsTab,
    agentStats,
    providerCount,
    categoryCount,
    teamCount,
    tokenTrendDialogOpen,
    openTokenTrendDialog,
    username,
    providerHealthSummary,
    sessionActiveCount,
    sessionSparkline,
    runnerStats,
  };
}
