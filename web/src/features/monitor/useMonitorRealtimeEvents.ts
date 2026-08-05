import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { storeToRefs } from 'pinia';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import { useMonitorStore } from '../../stores/monitor';
import { listMonitorTraces } from './api';
import type { MonitorTrace, StreamState } from './types';
import {
  findRunByTraceId,
  findRunByUsageEventId,
  isRunnerCompletionRow,
  runnerCompletionMetaFromRow,
  shouldHideCompletionInEvents,
  shouldHideWsRunnerCompletion,
} from './runCorrelation';
import { persistedEventToView, wsEventToView, buildMonitorEventsQuery, type MonitorViewEvent } from './eventView';
import { useMonitorRunNavigation } from './useMonitorRunNavigation';

export type { MonitorViewEvent } from './eventView';
// 筛选选项/查询组装的真相源在 eventView.ts（纯函数，已单测）；此处复出口保持组件导入路径稳定
export { EVENT_TYPE_FILTERS } from './eventView';

/** pulse 条上限：实时条只表达"此刻"，超出即丢弃最旧 */
const PULSE_MAX = 50;
const HISTORY_PAGE_SIZE = 15;

export function useMonitorRealtimeEvents() {
  const { t } = useI18n();
  const monitorStore = useMonitorStore();
  const { events: persistedRows, eventsTotal, eventsPaused: paused } = storeToRefs(monitorStore);
  const { openChatSession, openRunsTab } = useMonitorRunNavigation();

  // ── Pulse（WS 实时条：不落库、页面刷新即重建）──
  const pulseEvents = ref<MonitorViewEvent[]>([]);
  const state = ref<StreamState>('connecting');
  let wsSub: ReturnType<typeof monitorStore.startRuntimeEventsStream> | null = null;
  let hasRuntimeEvent = false;
  /** 单调递增序号：WS 事件 id 稳定性（旧实现用数组长度，会随推流漂移） */
  let wsSeq = 0;

  // ── completion ↔ Runs 关联数据（仅启动时拉取一次，用于降级判断/下钻）──
  const correlationTraces = ref<MonitorTrace[]>([]);
  async function loadCorrelationTraces() {
    try {
      const { items } = await listMonitorTraces({ limit: 200 });
      correlationTraces.value = items;
    } catch {
      /* 关联失败不阻断事件展示 */
    }
  }

  // ── 历史（monitor_events 持久化行：服务端分页 + 类型/级别过滤）──
  const historyLoading = ref(false);
  const page = ref(1);
  const pageSize = ref(HISTORY_PAGE_SIZE);
  const typeFilter = ref<string>('all');
  const severityFilter = ref<string>('all');
  // 默认隐藏 skill 文件同步等高频系统噪音；打开后全量展示。
  const showSystemEvents = ref(false);

  const historyEvents = computed<MonitorViewEvent[]>(() =>
    persistedRows.value
      .filter((row) => {
        if (!isRunnerCompletionRow(row)) return true;
        return !shouldHideCompletionInEvents(runnerCompletionMetaFromRow(row), correlationTraces.value);
      })
      .map((row) => persistedEventToView(t, row, correlationTraces.value)),
  );

  async function refreshHistory() {
    historyLoading.value = true;
    try {
      await monitorStore.loadEvents(
        buildMonitorEventsQuery({
          type: typeFilter.value,
          severity: severityFilter.value,
          page: page.value,
          pageSize: pageSize.value,
          includeSystem: showSystemEvents.value,
        }),
      );
    } finally {
      historyLoading.value = false;
    }
  }

  // 单一 watcher：过滤条件变化先归第 1 页（链式触发本轮），翻页/页大小变化仅重新查询——
  // 避免「过滤 + 非第 1 页」时双 watcher 各发一次相同请求
  watch(
    [typeFilter, severityFilter, showSystemEvents, page, pageSize],
    ([type, severity, showSystem], [oldType, oldSeverity, oldShowSystem]) => {
      const filterChanged = type !== oldType || severity !== oldSeverity || showSystem !== oldShowSystem;
      if (filterChanged && page.value !== 1) {
        page.value = 1;
        return;
      }
      void refreshHistory();
    },
  );

  // ── WS 流生命周期（暂停 = 断开订阅，恢复 = 重连）──
  function refreshStreamState() {
    if (paused.value) {
      state.value = 'paused';
      return;
    }
    if (!wsSub?.connected.value) {
      state.value = 'connecting';
      return;
    }
    state.value = hasRuntimeEvent ? 'live' : 'connected';
  }

  function startStream() {
    stopStream();
    hasRuntimeEvent = false;
    state.value = 'connecting';
    wsSub = monitorStore.startRuntimeEventsStream(
      GLOBAL_WS_SESSION_ID,
      (event) => {
        // WS runner_completion 与持久化行重复（chat 每次对话都双写），pulse 不展示；
        // 降级场景由历史表的 runner.completion 卡片承担
        if (shouldHideWsRunnerCompletion(event.type)) return;
        hasRuntimeEvent = true;
        state.value = 'live';
        wsSeq += 1;
        pulseEvents.value = [wsEventToView(t, event, wsSeq), ...pulseEvents.value].slice(0, PULSE_MAX);
      },
      () => {
        if (!paused.value) state.value = 'error';
      },
      () => {
        if (!paused.value) refreshStreamState();
      },
      () => {
        if (!paused.value) state.value = 'error';
      },
    );
  }

  function stopStream() {
    wsSub?.close();
    wsSub = null;
  }

  function toggleStream() {
    monitorStore.setEventsPaused(!paused.value);
  }

  function clearPulse() {
    pulseEvents.value = [];
  }

  /** 打开关联 Run（Traces Tab）：usage_event_id 优先，trace_id 兜底 */
  function openLinkedRun(event: MonitorViewEvent) {
    const meta = event.completionMeta;
    if (!meta) return;
    const rows = correlationTraces.value;
    const byUsage = meta.usage_event_id ? findRunByUsageEventId(rows, meta.usage_event_id) : undefined;
    const byTrace = meta.trace_id ? findRunByTraceId(rows, meta.trace_id) : undefined;
    const run = byUsage || byTrace;
    openRunsTab({
      session: meta.session_id,
      trace: meta.trace_id,
      usageEventId: run?.id || meta.usage_event_id,
    });
  }

  onMounted(() => {
    startStream();
    void loadCorrelationTraces();
    void refreshHistory();
  });
  onBeforeUnmount(stopStream);

  watch(paused, (isPaused) => {
    if (isPaused) {
      stopStream();
      state.value = 'paused';
    } else {
      startStream();
    }
  });

  return {
    // pulse
    pulseEvents,
    state,
    paused,
    toggleStream,
    clearPulse,
    // history
    historyEvents,
    historyTotal: eventsTotal,
    historyLoading,
    page,
    pageSize,
    typeFilter,
    severityFilter,
    showSystemEvents,
    refreshHistory,
    // actions
    openChatSession,
    openLinkedRun,
  };
}
