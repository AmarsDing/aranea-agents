/**
 * Harness「大屏显示」（/overview）：一屏看钱花在哪——各模型 / 各工具的消耗、费用、积分与额度；次要看执行任务量。
 *
 * 数据来源（除平台积分外都是已有接口）：
 * - 费用、Token、模型调用、按模型 / 按 Agent、趋势：usage 服务 GetUsageOverview（窗口跟随页头切换）
 * - 工具消耗：工具列表自带的调用统计（后端固定近 90 天窗口，另给近 24 小时调用数）
 * - 积分与额度：预算额度（quota_dashboard）+ 各平台积分（/v1/fleet/credits：DeepSeek 官方余额接口、Codex 本机日志，
 *   其余平台没有可合规调用的接口，由用户在这里手填）+ 编码终端日志里的额度窗口（/v1/fleet/usage）
 * - 执行任务量：Runner 近 24 小时指标 + 各状态任务数（任务态势接口 limit=1 只取 total）
 * 没有权限的不发请求；拿不到的数写「—」，不显示成 0。
 */
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { useUsageStore } from '../../stores/usage';
import { useMonitorStore } from '../../stores/monitor';
import { useNavStore } from '../../stores/nav';
import { listTools } from '../tools/api';
import type { Tool, ToolSummary } from '../tools/types';
import { listTaskSituations } from '../session/api';
import { listCredits, setCredit, type AgentUsage, type PlatformCredit } from '../fleet/api';
import { useTerminalUsage } from './useTerminalUsage';
import { shouldWarnZeroCost } from './pricingWarning';
import { formatCompactCount, formatCount, formatLatencyMs, formatUsdCompact, formatUsdFromMicro } from './moneyFormat';
import { listModelUsageTrends } from './api';
import type { ModelUsageBreakdownRow, ModelUsageTrendPoint, UsageQuotaDashboard } from './types';
import type { OvTone } from '../../components/overview/ovTypes';
import type {
  HsCreditForm,
  HsCreditRow,
  HsKpi,
  HsRange,
  HsShareRow,
  HsStat,
  HsTaskBar,
  HsToolRow,
} from '../../components/screen/hsTypes';

type T = (key: string, named?: Record<string, unknown>) => string;

const REFRESH_MS = 60_000;
const SHARE_TOP_N = 5;
const TOOL_TOP_N = 8;
export const TASK_STATUSES = ['running', 'waiting', 'queued', 'failed', 'completed'] as const;
type TaskStatus = (typeof TASK_STATUSES)[number];

const pct1 = (ratio: number) => `${(ratio * 100).toFixed(1)}%`;
/** 运行耗时常在几十秒：≥1 秒按秒写，免得出现「48200 ms」。 */
const duration = (ms?: number) => (ms != null && ms >= 1000 ? `${(ms / 1000).toFixed(1)} s` : formatLatencyMs(ms));

function rateTone(rate: number): OvTone {
  if (rate >= 0.99) return 'ok';
  if (rate >= 0.95) return 'warn';
  return 'danger';
}

/** 额度用量的颜色：≥90% 红、≥70% 琥珀（与终端额度条同一口径）。 */
export function usedTone(pct: number): OvTone {
  if (pct >= 90) return 'danger';
  if (pct >= 70) return 'warn';
  return 'ok';
}

type Amounts = { cost: number; tokens: number; calls: number };
const amountsOf = (r: { total_cost_micro_usd?: number; total_tokens?: number; call_count?: number }): Amounts => ({
  cost: r.total_cost_micro_usd ?? 0,
  tokens: r.total_tokens ?? 0,
  calls: r.call_count ?? 0,
});

/**
 * 前 5 名 + 「其余」一行。按费用排、按费用算占比；整段都没定价（费用为 0）时改按 Token。
 * 分母用整个周期的合计（total），不用返回行相加：概览接口最多只给前 8 个模型，相加会把占比算大。
 * 「其余」= 合计 − 前 5，包括接口没返回的模型和没归到 Agent 的调用。total 缺省时退回返回行之和。
 */
export function shareRows(
  source: ModelUsageBreakdownRow[],
  labelOf: (r: ModelUsageBreakdownRow) => string,
  t: T,
  total?: { total_cost_micro_usd?: number; total_tokens?: number; call_count?: number } | null,
) {
  const sumOf = (list: Amounts[]) =>
    list.reduce((a, b) => ({ cost: a.cost + b.cost, tokens: a.tokens + b.tokens, calls: a.calls + b.calls }), {
      cost: 0,
      tokens: 0,
      calls: 0,
    });
  const grand = total ? amountsOf(total) : sumOf(source.map(amountsOf));
  const byCost = grand.cost > 0;
  const weight = (a: Amounts) => (byCost ? a.cost : a.tokens);
  // 只画有消耗的行：只有调用次数、Token 和费用都是 0 的行画进环图会占满一圈，误导
  const rows = source.map((r) => ({ r, a: amountsOf(r) })).filter((x) => weight(x.a) > 0);
  rows.sort((x, y) => weight(y.a) - weight(x.a));
  const denom = weight(grand);
  const row = (key: string, label: string, a: Amounts): HsShareRow => ({
    key,
    label,
    cost: formatUsdCompact(a.cost),
    tokens: formatCompactCount(a.tokens),
    calls: formatCount(a.calls),
    share: denom > 0 ? weight(a) / denom : 0,
    unpriced: shouldWarnZeroCost({ totalTokens: a.tokens, totalCostMicroUsd: a.cost }),
  });
  const top = rows.slice(0, SHARE_TOP_N);
  const out = top.map((x, i) => row(`${i}:${labelOf(x.r)}`, labelOf(x.r) || '—', x.a));
  const topSum = sumOf(top.map((x) => x.a));
  const rest = {
    cost: Math.max(grand.cost - topSum.cost, 0),
    tokens: Math.max(grand.tokens - topSum.tokens, 0),
    calls: Math.max(grand.calls - topSum.calls, 0),
  };
  if (weight(rest) > 0) out.push(row('__others', t('harnessScreen.othersRest'), rest));
  return { rows: out, byCost };
}

/**
 * 昨日同时段的用量：今天（UTC 日）已过的小时，对应昨天同样的小时；当前这一小时按已过的分钟折算。
 * 「今日」还没过完，拿去和昨天全天比会一早就显示大跌，所以环比用这个。小时键形如 2026-11-19T08（UTC）。
 */
export function sameTimeYesterday(points: ModelUsageTrendPoint[], now: Date): Amounts {
  const h = now.getUTCHours();
  const frac = now.getUTCMinutes() / 60;
  const out = { cost: 0, tokens: 0, calls: 0 };
  for (const p of points) {
    const m = /T(\d{2})/.exec(p.date_key);
    if (!m) continue;
    const hh = Number(m[1]);
    const w = hh < h ? 1 : hh === h ? frac : 0;
    out.cost += (p.total_cost_micro_usd ?? 0) * w;
    out.tokens += (p.total_tokens ?? 0) * w;
    out.calls += (p.call_count ?? 0) * w;
  }
  return out;
}

/** UTC 小时键（2026-11-19T08）→ 本地「HH:00」。 */
export function localHourLabel(key: string) {
  const m = /^(\d{4}-\d{2}-\d{2})T(\d{2})/.exec(key);
  if (!m) return key;
  return `${pad2(new Date(`${m[1]}T${m[2]}:00:00Z`).getHours())}:00`;
}

/** 周期起点（UTC），与后端 usage 的 normalizeQuery 一致：今日 / 近 7 天 / 近 30 天 / 本月，都从 UTC 零点算。 */
export function rangeStartUTC(range: HsRange, now: Date): Date {
  const y = now.getUTCFullYear();
  const m = now.getUTCMonth();
  const d = now.getUTCDate();
  if (range === 'month') return new Date(Date.UTC(y, m, 1));
  const back = range === '7d' ? 6 : range === '30d' ? 29 : 0;
  return new Date(Date.UTC(y, m, d - back));
}

/** 月底预估（微美元）：本月已花 ÷ 已过天数 × 本月天数（UTC）。本月不到 3 天时样本太少，返回 null。 */
export function monthEndForecast(monthCostMicro: number, now: Date): number | null {
  const start = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1);
  const days = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() + 1, 0)).getUTCDate();
  const elapsed = (now.getTime() - start) / 86_400_000;
  if (elapsed < 3) return null;
  return (monthCostMicro / elapsed) * days;
}

export function toolRows(tools: Tool[]): HsToolRow[] {
  const max = Math.max(...tools.map((x) => x.invoke_count), 1);
  return tools.map((x) => {
    const calls = Math.max(x.invoke_count, 1);
    return {
      key: x.key,
      label: x.display_name || x.key,
      calls: formatCount(x.invoke_count),
      ratio: x.invoke_count / max,
      okPart: x.success_count / calls,
      failPart: x.failure_count / calls,
      blockPart: x.blocked_count / calls,
      failRate: x.invoke_count > 0 ? pct1(x.failure_count / x.invoke_count) : '—',
    };
  });
}

/** 平台展示名（专有名词，两种语言一样）。 */
export const PLATFORM_NAMES: Record<string, string> = {
  deepseek: 'DeepSeek',
  codex: 'Codex',
  claude: 'Claude',
  codebuddy: 'CodeBuddy / WorkBuddy',
  qoder: 'Qoder',
  trae: 'Trae CN',
  cursor: 'Cursor',
};

const pad2 = (v: number) => String(v).padStart(2, '0');
function shortTime(iso: string) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return `${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

function amount(v: number, unit: string, t: T) {
  const num = new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 2 }).format(v);
  if (unit === 'CNY') return `¥${num}`;
  if (unit === 'USD') return `$${num}`;
  return t('harnessScreen.credits.amount', { value: num });
}

/** 额度窗口标签形如 5h / 7d / 30d。 */
function windowLabel(label: string, t: T) {
  const m = /^(\d+)([hd])$/.exec(label);
  return m ? t(`harnessScreen.quota.win.${m[2]}`, { n: Number(m[1]) }) : label;
}

/**
 * 积分与额度的行：预算（配了才有）+ 每个平台一行。credits：undefined 还在取，null 取不到。
 * 终端日志里已知百分比的额度窗口挂在对应平台行上（claude / codex）。
 */
/** 手填超过这么多天没更新就提醒「可能过期」。 */
const STALE_DAYS = 7;

/**
 * 近 days 天的消耗：窗口内（含窗口前一天作起点）相邻两天剩余下降之和；上涨按充值处理，不算消耗。
 * 快照少于 2 个点时返回 null（还算不出来）。日期是 UTC 日。
 */
export function consumedIn(history: Array<{ date: string; remaining: number }>, days: number, now: Date) {
  const cutoff = new Date(now.getTime() - days * 86_400_000).toISOString().slice(0, 10);
  const pts = history.filter((p) => p.date >= cutoff);
  if (pts.length < 2) return null;
  let used = 0;
  for (let i = 1; i < pts.length; i++) used += Math.max(pts[i - 1].remaining - pts[i].remaining, 0);
  return used;
}

export function creditRows(
  quota: UsageQuotaDashboard | null | undefined,
  credits: PlatformCredit[] | null | undefined,
  terminals: AgentUsage[] | null | undefined,
  t: T,
  now: Date = new Date(),
): HsCreditRow[] {
  const rows: HsCreditRow[] = [];
  if (quota && quota.configured_count > 0) {
    const used = quota.total_cap_micro_usd > 0 ? quota.total_spent_micro_usd / quota.total_cap_micro_usd : 0;
    rows.push({
      key: 'budget',
      title: t('harnessScreen.quota.budget'),
      sub: t('harnessScreen.quota.budgetSub', { count: quota.configured_count }),
      value: `${formatUsdFromMicro(quota.total_spent_micro_usd, 2)} / ${formatUsdFromMicro(quota.total_cap_micro_usd, 2)}`,
      used,
      tone: usedTone(used * 100),
      windows: [],
      editable: false,
      missing: false,
      spark: [],
      stale: false,
    });
  }
  for (const c of credits ?? []) {
    const windows = (terminals ?? [])
      .filter((term) => term.agent === c.platform)
      .flatMap((term) => term.windows.filter((w) => w.percent >= 0))
      .map((w) => ({ label: windowLabel(w.label, t), pct: w.percent, tone: usedTone(w.percent) }));
    const unset = c.source === 'manual' && !c.updatedAt;
    const known = c.remaining >= 0;
    const used = c.total > 0 && known ? Math.min(Math.max((c.total - c.remaining) / c.total, 0), 1) : null;
    const consumed = consumedIn(c.history ?? [], 7, now);
    const stale =
      c.source === 'manual' && !unset && now.getTime() - new Date(c.updatedAt).getTime() > STALE_DAYS * 86_400_000;
    const sub = [
      stale && t('harnessScreen.credits.stale'),
      consumed !== null && t('harnessScreen.credits.used7d', { value: amount(consumed, c.unit, t) }),
      c.plan && t('harnessScreen.credits.plan', { plan: c.plan }),
      c.total > 0 && t('harnessScreen.credits.of', { total: amount(c.total, c.unit, t) }),
      c.note,
      c.source === 'manual' && !unset && t('harnessScreen.credits.filledAt', { time: shortTime(c.updatedAt) }),
    ].filter(Boolean);
    rows.push({
      key: c.platform,
      title: PLATFORM_NAMES[c.platform] ?? c.platform,
      // 没登记的手填行不写提示（标题栏已说明「其余手填」），省出一行高度好一屏放下
      sub: unset ? '' : sub.join(' · ') || t(`harnessScreen.credits.source.${c.source}`),
      value: unset
        ? t('harnessScreen.credits.unset')
        : c.unlimited
          ? t('harnessScreen.credits.unlimited')
          : known
            ? amount(c.remaining, c.unit, t)
            : '—',
      used,
      tone: used !== null ? usedTone(used * 100) : 'neutral',
      windows,
      editable: c.source === 'manual',
      platform: c.platform,
      missing: unset || (!known && !c.unlimited),
      spark: (c.history ?? []).length >= 2 ? c.history.map((p) => p.remaining) : [],
      stale,
    });
  }
  return rows;
}

export function useHarnessScreen() {
  const { t } = useI18n();
  const usageStore = useUsageStore();
  const { overview, error: usageError } = storeToRefs(usageStore);
  const monitorStore = useMonitorStore();
  const { runnerMetrics } = storeToRefs(monitorStore);
  const nav = useNavStore();
  // 没有运行台权限就不发请求，免得弹一条 403 提示
  const { terminals, refresh: refreshTerminals } = useTerminalUsage(() => nav.isPathVisible('/fleet'));

  const range = ref<HsRange>('today');
  const mode = ref<'model' | 'agent'>('model');
  const loading = ref(false);
  const updatedAt = ref<Date | null>(null);
  /** undefined = 还没取回；null = 取不到或没有权限。 */
  const tools = ref<Tool[] | null>();
  const toolSummary = ref<ToolSummary | null>(null);
  const taskCounts = ref<Partial<Record<TaskStatus, number>>>({});
  const runsAvailable = ref(true);
  /** 平台积分；undefined = 还没取回；null = 取不到或没有权限。 */
  const credits = ref<PlatformCredit[] | null>();
  /** 下钻地址：看不到的页面不给链接，免得点进 403。 */
  const link = (path: string) => (nav.isPathVisible(path) ? path : undefined);
  const links = computed(() => ({ usage: link('/usage/events'), tools: link('/tools'), tasks: link('/tasks') }));
  /** 昨日（UTC 日）各小时用量，只在「今日」时取；null = 没取或取不到。 */
  const yesterdayHours = ref<ModelUsageTrendPoint[] | null>(null);

  const rangeOptions = computed(() =>
    (['today', '7d', '30d', 'month'] as const).map((value) => ({ value, label: t(`harnessScreen.range.${value}`) })),
  );

  async function loadUsage() {
    const today = range.value === 'today';
    // 「今日」要和昨日同时段比，另取昨日（UTC 日）的小时数据；其余周期不比
    const yday = new Date(Date.now() - 86_400_000).toISOString().slice(0, 10);
    const [, y] = await Promise.allSettled([
      usageStore.loadOverview({ range: range.value }, today ? 'hour' : 'day'),
      today ? listModelUsageTrends({ start_date: yday, end_date: yday, granularity: 'hour' }) : Promise.resolve(null),
    ]);
    // 概览失败时 store 里有 error，页面显示；昨日小时数据失败只是不显示对比
    yesterdayHours.value = y.status === 'fulfilled' ? y.value : null;
  }

  async function loadTools() {
    if (!nav.isPathVisible('/tools')) {
      tools.value = null;
      return;
    }
    try {
      const res = await listTools({ sort: 'invoke_count', page: 1, page_size: TOOL_TOP_N });
      tools.value = res.items.filter((x) => x.invoke_count > 0);
      toolSummary.value = res.summary ?? null;
    } catch (err) {
      console.warn('[screen] tools failed', err);
      tools.value = null;
    }
  }

  async function loadRuns() {
    try {
      // 运行次数跟随页头周期（Runner 指标按分钟窗口取，从周期起点算到现在）
      const now = new Date();
      await monitorStore.loadRunnerMetrics(
        Math.max(1, Math.ceil((now.getTime() - rangeStartUTC(range.value, now).getTime()) / 60_000)),
      );
      runsAvailable.value = true;
    } catch (err) {
      console.warn('[screen] runner metrics failed', err);
      runsAvailable.value = false;
    }
  }

  async function loadTasks() {
    const next: Partial<Record<TaskStatus, number>> = {};
    await Promise.all(
      TASK_STATUSES.map(async (status) => {
        try {
          next[status] = (await listTaskSituations({ status, limit: 1 })).total;
        } catch (err) {
          console.warn(`[screen] ${status} tasks failed`, err);
        }
      }),
    );
    taskCounts.value = next;
  }

  // 积分接口与终端用量同属运行台（coding.manage）
  async function loadCredits() {
    if (!nav.isPathVisible('/fleet')) {
      credits.value = null;
      return;
    }
    try {
      credits.value = await listCredits();
    } catch (err) {
      console.warn('[screen] credits failed', err);
      credits.value = null;
    }
  }

  /** 打开手填表单需要的当前值。 */
  function creditForm(platform: string): HsCreditForm {
    const c = credits.value?.find((x) => x.platform === platform);
    const filled = !!c?.updatedAt;
    return {
      platform,
      title: PLATFORM_NAMES[platform] ?? platform,
      plan: c?.plan ?? '',
      total: filled && c!.total >= 0 ? c!.total : null,
      remaining: filled && c!.remaining >= 0 ? c!.remaining : null,
      unit: c?.unit || 'credits',
      note: c?.note ?? '',
    };
  }

  /** 保存手填积分；失败抛给调用方显示。 */
  async function saveCredit(form: HsCreditForm) {
    const saved = await setCredit({
      platform: form.platform,
      plan: form.plan,
      total: form.total ?? -1,
      remaining: form.remaining ?? -1,
      unit: form.unit,
      note: form.note,
    });
    credits.value = (credits.value ?? []).map((c) => (c.platform === saved.platform ? saved : c));
  }

  async function load(withTerminals: boolean) {
    loading.value = true;
    await Promise.allSettled([
      loadUsage(),
      loadTools(),
      loadRuns(),
      loadTasks(),
      loadCredits(),
      ...(withTerminals ? [refreshTerminals()] : []),
    ]);
    loading.value = false;
    updatedAt.value = new Date();
  }
  const refresh = () => load(true);

  function setRange(next: HsRange) {
    range.value = next;
    void loadUsage();
    void loadRuns();
  }

  // 大屏常开：每分钟刷新一次，页面在后台时跳过
  let timer: ReturnType<typeof setInterval> | undefined;
  onMounted(() => {
    void load(false); // 终端用量由 useTerminalUsage 自己在挂载时取
    timer = setInterval(() => {
      if (!document.hidden && !loading.value) void refresh();
    }, REFRESH_MS);
  });
  onUnmounted(() => clearInterval(timer));

  // —— 派生 ——
  const summary = computed(() => overview.value?.range ?? null);
  const trends = computed(() => overview.value?.trends ?? []);
  const isToday = computed(() => range.value === 'today');
  /** 昨日同时段的用量；只在「今日」且取到了昨日小时数据时有值。 */
  const sameTime = computed(() =>
    isToday.value && yesterdayHours.value ? sameTimeYesterday(yesterdayHours.value, new Date()) : null,
  );
  const vsSameTime = (cur: number | undefined, pick: (a: Amounts) => number) => {
    const prev = sameTime.value ? pick(sameTime.value) : 0;
    return prev > 0 ? ((cur ?? 0) - prev) / prev : null;
  };

  const shares = computed(() =>
    mode.value === 'model'
      ? shareRows(overview.value?.top_models ?? [], (r) => r.model_display_name || r.model_api_id, t, summary.value)
      : shareRows(overview.value?.top_agents ?? [], (r) => r.agent_key || r.agent_id, t, summary.value),
  );
  const totalCost = computed(() => (summary.value ? formatUsdCompact(summary.value.total_cost_micro_usd) : '—'));
  /** 有 Token 却没记到钱的模型：费用数字会偏低，要在费用卡上说出来。 */
  const unpriced = computed(
    () =>
      (overview.value?.top_models ?? []).some((r) =>
        shouldWarnZeroCost({ totalTokens: r.total_tokens, totalCostMicroUsd: r.total_cost_micro_usd }),
      ) ||
      (!!summary.value &&
        shouldWarnZeroCost({
          totalTokens: summary.value.total_tokens,
          totalCostMicroUsd: summary.value.total_cost_micro_usd,
        })),
  );

  const tools8 = computed(() => (tools.value ? toolRows(tools.value) : null));
  const creditList = computed(() => creditRows(overview.value?.quota_dashboard, credits.value, terminals.value, t));

  /** 积分 / 预算卡：只看积分和预算的已用比例（终端调用额度窗口是限速，不是积分，不算进来）。 */
  const topCredit = computed(() => {
    let best: HsCreditRow | null = null;
    for (const row of creditList.value) if (row.used !== null && (!best || row.used > (best.used ?? 0))) best = row;
    return best;
  });

  const kpis = computed<HsKpi[]>(() => {
    const s = summary.value;
    const calls = s?.call_count ?? 0;
    const ts = toolSummary.value;
    const top = topCredit.value;
    const st = sameTime.value;
    const monthCost = overview.value?.month?.total_cost_micro_usd ?? 0;
    const monthForecast = overview.value?.month ? monthEndForecast(monthCost, new Date()) : null;
    const costCaption = !s
      ? t('harnessScreen.unavailable')
      : unpriced.value
        ? t('harnessScreen.kpi.costUnpriced')
        : isToday.value
          ? st
            ? t('harnessScreen.kpi.costSameTime', { value: formatUsdCompact(st.cost) })
            : t('harnessScreen.kpi.noSameTime')
          : monthForecast !== null
            ? t('harnessScreen.kpi.costMonthForecast', {
                value: formatUsdCompact(monthCost),
                forecast: formatUsdCompact(monthForecast),
              })
            : t('harnessScreen.kpi.costMonth', { value: formatUsdCompact(monthCost) });
    return [
      {
        key: 'cost',
        to: link(unpriced.value ? '/models' : '/usage/events'),
        label: t('harnessScreen.kpi.cost'),
        value: totalCost.value,
        caption: costCaption,
        delta: vsSameTime(s?.total_cost_micro_usd, (a) => a.cost),
        deltaGoodWhenNegative: true,
        tone: unpriced.value ? 'warn' : 'info',
        spark: trends.value.map((p) => p.total_cost_micro_usd ?? 0),
      },
      {
        key: 'tokens',
        to: link('/usage/events'),
        label: t('harnessScreen.kpi.tokens'),
        value: s ? formatCompactCount(s.total_tokens) : '—',
        caption: s
          ? t('harnessScreen.kpi.tokensSplit', {
              input: formatCompactCount(s.input_tokens),
              output: formatCompactCount(s.output_tokens),
            })
          : t('harnessScreen.unavailable'),
        delta: vsSameTime(s?.total_tokens, (a) => a.tokens),
        tone: 'info',
        spark: trends.value.map((p) => p.total_tokens ?? 0),
      },
      {
        key: 'calls',
        to: link('/usage/events'),
        label: t('harnessScreen.kpi.calls'),
        value: s ? formatCount(calls) : '—',
        caption:
          calls > 0 ? t('harnessScreen.kpi.successRate', { value: pct1(s!.success_rate) }) : t('harnessScreen.noCalls'),
        delta: vsSameTime(s?.call_count, (a) => a.calls),
        tone: calls > 0 ? rateTone(s!.success_rate) : 'neutral',
        spark: trends.value.map((p) => p.call_count ?? 0),
      },
      {
        key: 'tools',
        to: link('/tools'),
        label: t('harnessScreen.kpi.tools'),
        value: ts ? formatCount(ts.calls_24h) : '—',
        caption: ts
          ? t('harnessScreen.kpi.toolsFail', { value: pct1(ts.failure_rate_24h), count: ts.enabled_tools })
          : t('harnessScreen.unavailable'),
        tone: ts && ts.calls_24h > 0 ? rateTone(1 - ts.failure_rate_24h) : 'neutral',
      },
      {
        key: 'quota',
        label: t('harnessScreen.kpi.quota'),
        value: top ? `${Math.round((top.used ?? 0) * 100)}%` : '—',
        caption: top ? top.title : t('harnessScreen.kpi.quotaNone'),
        tone: top ? top.tone : 'neutral',
      },
    ];
  });

  // 后端按 UTC 切小时；「今日」的小时标签换成本地时间，日标签仍是 UTC 日期（页头写明）
  const trendLabels = computed(() =>
    trends.value.map((p) => (isToday.value ? localHourLabel(p.date_key) : p.date_key.slice(5, 10))),
  );
  const trendCalls = computed(() => trends.value.map((p) => p.call_count ?? 0));
  const trendCosts = computed(() => trends.value.map((p) => (p.total_cost_micro_usd ?? 0) / 1_000_000));
  /** 「今日」（UTC 日）从本地几点开始，页头用。 */
  const dayStartLocal = computed(() => {
    void updatedAt.value; // 每次刷新重算，跨过 UTC 零点后跟着变
    const n = new Date();
    const d = new Date(Date.UTC(n.getUTCFullYear(), n.getUTCMonth(), n.getUTCDate()));
    return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
  });

  const runStats = computed<HsStat[]>(() => {
    const rm = runsAvailable.value ? runnerMetrics.value : null;
    const total = rm?.total_runs ?? 0;
    return [
      { key: 'runs', label: t('harnessScreen.task.runs'), value: rm ? formatCount(total) : '—' },
      {
        key: 'pass',
        label: t('harnessScreen.task.passRate'),
        value: total > 0 ? pct1(rm!.success_rate) : '—',
        tone: total > 0 ? rateTone(rm!.success_rate) : 'neutral',
      },
      {
        key: 'p95',
        label: t('harnessScreen.task.p95'),
        value: total > 0 ? duration(rm?.p95_duration_ms) : '—',
      },
    ];
  });

  const taskTotal = computed(() => TASK_STATUSES.reduce((acc, s) => acc + (taskCounts.value[s] ?? 0), 0));
  const taskBars = computed<HsTaskBar[]>(() =>
    TASK_STATUSES.filter((s) => taskCounts.value[s] !== undefined).map((s) => ({
      key: s,
      label: t(`harnessScreen.task.status.${s}`),
      count: taskCounts.value[s] ?? 0,
      share: taskTotal.value > 0 ? (taskCounts.value[s] ?? 0) / taskTotal.value : 0,
    })),
  );

  return {
    t,
    error: usageError,
    loading,
    updatedAt,
    range,
    rangeOptions,
    setRange,
    refresh,
    mode,
    kpis,
    shares,
    totalCost,
    tools: tools8,
    toolSummary,
    creditList,
    credits,
    creditForm,
    saveCredit,
    trendLabels,
    dayStartLocal,
    links,
    trendCalls,
    trendCosts,
    runStats,
    taskBars,
    taskTotal,
  };
}
