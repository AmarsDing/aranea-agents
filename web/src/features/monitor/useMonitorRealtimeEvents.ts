import { computed, onBeforeUnmount, onMounted, ref, watch, type Ref } from "vue";
import { copyToClipboard, Notify } from "quasar";
import { storeToRefs } from "pinia";
import { GLOBAL_WS_SESSION_ID } from "../../config/runtime";
import { useMonitorStore } from "../../stores/monitor/index";
import type { PlatformResource, StreamState, TeamRunEvent, MonitorTraceEvent } from "./types";
import {
  completionFallbackSubtitle,
  completionCanOpenInRuns,
  completionFallbackTitle,
  findRunByTraceId,
  findRunByUsageEventId,
  isRunnerCompletionRow,
  runnerCompletionMetaFromRow,
  shouldHideCompletionInEvents,
  shouldHideWsRunnerCompletion,
  type RunnerCompletionMeta
} from "./runCorrelation";
import { useMonitorRunNavigation } from "./useMonitorRunNavigation";
import { compactJSON, formatDate, parseJSON } from "./utils";

export type MonitorViewEvent = {
  id: string;
  type: string;
  title: string;
  subtitle: string;
  category: string;
  source: string;
  time: string;
  raw: unknown;
  completionMeta?: RunnerCompletionMeta;
  canOpenInRuns?: boolean;
  completionSessionId?: string;
};

export function useMonitorRealtimeEvents(
  persistedEvents: Ref<PlatformResource[]>,
  traces: Ref<MonitorTraceEvent[] | undefined>
) {
  const { openChatSession, openRunsTab } = useMonitorRunNavigation();
  const monitorStore = useMonitorStore();
  const { eventsPaused: paused } = storeToRefs(monitorStore);

  const category = ref("all");
  const state = ref<StreamState>("connecting");
  const runtimeEvents = ref<MonitorViewEvent[]>([]);
  const selected = ref<MonitorViewEvent | null>(null);
  const detailOpen = ref(false);

  let wsSub: ReturnType<typeof monitorStore.startRuntimeEventsStream> | null = null;
  let hasRuntimeEvent = false;

  const categoryOptions = [
    { label: "全部", value: "all" },
    { label: "任务", value: "task" },
    { label: "消息", value: "message" },
    { label: "Agent", value: "agent" },
    { label: "工具", value: "tool" },
    { label: "系统", value: "system" }
  ];

  const persistedViewEvents = computed<MonitorViewEvent[]>(() => {
    const traceRows = traces.value ?? [];
    return persistedEvents.value
      .filter((row) => {
        if (!isRunnerCompletionRow(row)) return true;
        const meta = runnerCompletionMetaFromRow(row);
        return !shouldHideCompletionInEvents(meta, traceRows);
      })
      .map((row) => {
        const cfg = parseJSON(row.config_json);
        const type = String(cfg.type || row.key || "monitor.event");
        const completion = isRunnerCompletionRow(row);
        const meta = completion ? runnerCompletionMetaFromRow(row) : undefined;
        return {
          id: row.id,
          type,
          title: completion ? completionFallbackTitle(meta!, row.name) : row.name || type,
          subtitle: completion ? completionFallbackSubtitle(meta!, row.description) : row.description || JSON.stringify(cfg),
          category: categoryFor(type),
          source: "persisted",
          time: formatDate(row.created_at),
          raw: { ...row, config: cfg, metadata: parseJSON(row.metadata_json) },
          completionMeta: meta,
          canOpenInRuns: completion && meta ? completionCanOpenInRuns(meta, traceRows) : false,
          completionSessionId: completion ? String(meta?.session_id || "").trim() || undefined : undefined
        };
      });
  });

  const visibleEvents = computed(() => {
    const wsItems = runtimeEvents.value.filter((ev) => !shouldHideWsRunnerCompletion(ev.type));
    const items = [...wsItems, ...persistedViewEvents.value];
    if (category.value === "all") return items;
    return items.filter((event) => event.category === category.value);
  });

  const selectedJSON = computed(() => compactJSON(selected.value?.raw ?? {}));
  const emptyHint = computed(() => {
    if (state.value === "connected") {
      return "已连接，等待运行时事件（发起 Team / Agent 运行后可看到实时推送）";
    }
    if (state.value === "connecting") {
      return "正在连接 WebSocket…";
    }
    return "暂无实时事件";
  });
  const streamTextMap: Record<StreamState, string> = {
    connecting: "连接中",
    connected: "已连接",
    live: "实时",
    paused: "已暂停",
    error: "连接异常"
  };
  const streamText = computed(() => streamTextMap[state.value]);
  const streamColor = computed(() => {
    const s = state.value;
    if (s === "live" || s === "connected") return "positive";
    if (s === "error") return "negative";
    if (s === "paused") return "grey";
    return "orange";
  });

  function categoryFor(type: string) {
    if (type === "intent_pass") return "agent";
    if (type.startsWith("run") || type.includes("team_run")) return "task";
    if (type.startsWith("message") || type.startsWith("chat")) return "message";
    if (type.startsWith("agent")) return "agent";
    if (type.startsWith("tool") || type.includes("step")) return "tool";
    return "system";
  }

  function eventColor(type: string) {
    if (type.includes("failed") || type.includes("error")) return "negative";
    if (type.includes("finished") || type.includes("completed")) return "positive";
    if (type === "intent_pass") return "cyan";
    if (type.includes("tool") || type.includes("step")) return "orange";
    if (type.includes("agent")) return "cyan";
    return "primary";
  }

  function refreshStreamState() {
    if (paused.value) {
      state.value = "paused";
      return;
    }
    if (!wsSub?.connected.value) {
      state.value = "connecting";
      return;
    }
    state.value = hasRuntimeEvent ? "live" : "connected";
  }

  function teamRunEventToView(event: TeamRunEvent): MonitorViewEvent {
    const type = event.type || "runtime.event";
    const step = event.step;
    const run = event.run;
    const payload = event.payload || {};
    let title = step?.agent_name || run?.mode || type;
    let subtitle = step?.error_message || run?.error_message || step?.output_preview || run?.output_preview || "runtime event";
    if (type === "intent_pass") {
      title = "意图 Pass";
      const outcome = String(payload.outcome ?? "");
      const ms = payload.duration_ms;
      subtitle =
        outcome +
        (typeof ms === "number" ? ` · ${ms} ms` : "") +
        (payload.intent_kind ? ` · ${String(payload.intent_kind)}` : "");
    }
    return {
      id: `${type}-${run?.id || step?.id || Date.now()}-${runtimeEvents.value.length}`,
      type,
      title,
      subtitle,
      category: categoryFor(type),
      source: "ws",
      time: formatDate(step?.created_at || run?.updated_at || run?.created_at || new Date().toISOString()),
      raw: event
    };
  }

  function startStream() {
    stopStream();
    hasRuntimeEvent = false;
    state.value = "connecting";
    wsSub = monitorStore.startRuntimeEventsStream(
      GLOBAL_WS_SESSION_ID,
      (event) => {
        hasRuntimeEvent = true;
        state.value = "live";
        runtimeEvents.value = [teamRunEventToView(event), ...runtimeEvents.value].slice(0, 1000);
      },
      () => {
        if (!paused.value) state.value = "error";
      },
      () => {
        if (!paused.value) refreshStreamState();
      },
      () => {
        if (!paused.value) state.value = "error";
      }
    );
  }

  function stopStream() {
    wsSub?.close();
    wsSub = null;
  }

  function toggleStream() {
    monitorStore.setEventsPaused(!paused.value);
  }

  function clearRuntimeEvents() {
    runtimeEvents.value = [];
  }

  function openDetail(event: MonitorViewEvent) {
    selected.value = event;
    detailOpen.value = true;
  }

  function openLinkedRun(event: MonitorViewEvent) {
    const meta = event.completionMeta;
    if (!meta) return;
    const traceRows = traces.value ?? [];
    const byUsage = meta.usage_event_id ? findRunByUsageEventId(traceRows, meta.usage_event_id) : undefined;
    const byTrace = meta.trace_id ? findRunByTraceId(traceRows, meta.trace_id) : undefined;
    const run = byUsage || byTrace;
    openRunsTab({
      session: meta.session_id,
      trace: meta.trace_id,
      usageEventId: run?.id || meta.usage_event_id
    });
  }

  async function copyJSON() {
    await copyToClipboard(selectedJSON.value);
    Notify.create({ message: "已复制", color: "positive", position: "top" });
  }

  onMounted(startStream);
  onBeforeUnmount(stopStream);

  watch(paused, (isPaused) => {
    if (isPaused) {
      stopStream();
      state.value = "paused";
    } else {
      startStream();
    }
  });

  return {
    category,
    categoryOptions,
    state,
    paused,
    runtimeEvents,
    selected,
    detailOpen,
    visibleEvents,
    selectedJSON,
    emptyHint,
    streamText,
    streamColor,
    toggleStream,
    clearRuntimeEvents,
    openDetail,
    openLinkedRun,
    openChatSession,
    copyJSON,
    eventColor
  };
}
