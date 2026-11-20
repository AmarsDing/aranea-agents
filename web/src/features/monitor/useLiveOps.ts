/**
 * 实时运转页（/monitor/live，2026-11-19 重做）取数：回答「现在谁在干什么」。
 *  - 正在跑的运行：GET /v1/monitor/traces?status=running（默认排除 system / skill 内部噪音）
 *  - 刚刚结束的运行：同一接口最新 10 条
 *  - 编码终端：运行台 GET /v1/fleet/terminals（需 coding.manage，没有就不读）
 *  - 等人确认：收件箱汇总 GET /v1/inbox/summary（处理在 工作台 › 今日，这里只给数和入口）
 *  - 近 1 小时完成 / 失败：GET /v1/monitor/runner-metrics?windowMinutes=60
 * 每块独立读，失败只记在那一块；运行开始 / 结束的 WS 事件到达时刷新运行列表，另外每 15 秒兜底一次。
 */
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useNavStore } from '../../stores/nav';
import { listMonitorTraces, getRunnerMetrics } from './api';
import { useMonitorRunsLive } from './useMonitorRunsLive';
import { listTerminals, type Terminal } from '../fleet/api';
import { getInboxSummary } from '../approval/api';
import type { MonitorTraceRow, RunnerMetricsSummary } from './types';

const POLL_MS = 15_000;
/** 运行台里算「在跑」的终端状态 */
const BUSY_TERMINAL = new Set(['starting', 'working', 'needs_input']);

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export function useLiveOps() {
  const nav = useNavStore();
  const canFleet = computed(() => nav.permissions.includes('*') || nav.permissions.includes('coding.manage'));

  const running = ref<MonitorTraceRow[]>([]);
  const runningTotal = ref(0);
  const recent = ref<MonitorTraceRow[]>([]);
  const runsError = ref('');
  const terminals = ref<Terminal[]>([]);
  const terminalsError = ref('');
  const inbox = ref<{ total: number; highRisk: number } | null>(null);
  const hour = ref<RunnerMetricsSummary | null>(null);
  const loaded = ref(false);

  async function loadRuns() {
    try {
      const [run, last] = await Promise.all([
        listMonitorTraces({ status: 'running', exclude_internal: true, limit: 20 }),
        listMonitorTraces({ exclude_internal: true, limit: 10 }),
      ]);
      running.value = run.items;
      runningTotal.value = run.total;
      recent.value = last.items.filter((r) => r.status !== 'running');
      runsError.value = '';
    } catch (err) {
      runsError.value = message(err);
    }
  }

  async function loadTerminals() {
    if (!canFleet.value) return;
    try {
      terminals.value = await listTerminals();
      terminalsError.value = '';
    } catch (err) {
      terminalsError.value = message(err);
    }
  }

  async function loadSide() {
    const [summary, metrics] = await Promise.allSettled([getInboxSummary(), getRunnerMetrics(60)]);
    if (summary.status === 'fulfilled') inbox.value = { total: summary.value.total, highRisk: summary.value.highRisk };
    if (metrics.status === 'fulfilled') hour.value = metrics.value;
  }

  let lastAt = 0;
  async function refresh() {
    lastAt = Date.now();
    await Promise.all([loadRuns(), loadTerminals(), loadSide()]);
    loaded.value = true;
  }

  const busyTerminals = computed(() =>
    terminals.value
      .filter((x) => BUSY_TERMINAL.has(x.status))
      .sort((a, b) => Number(b.status === 'needs_input') - Number(a.status === 'needs_input')),
  );

  // 运行开始 / 结束的 WS 事件（已防抖）→ 只刷运行列表
  const live = useMonitorRunsLive(() => void loadRuns());

  let timer: ReturnType<typeof setInterval> | null = null;
  function onVisible() {
    if (document.visibilityState === 'visible' && Date.now() - lastAt > POLL_MS) void refresh();
  }
  onMounted(() => {
    void refresh();
    timer = setInterval(() => {
      if (document.visibilityState === 'visible') void refresh();
    }, POLL_MS);
    document.addEventListener('visibilitychange', onVisible);
  });
  onUnmounted(() => {
    if (timer) clearInterval(timer);
    document.removeEventListener('visibilitychange', onVisible);
  });

  return {
    canFleet,
    running,
    runningTotal,
    recent,
    runsError,
    terminals,
    busyTerminals,
    terminalsError,
    inbox,
    hour,
    loaded,
    liveState: live.state,
    refresh,
  };
}
