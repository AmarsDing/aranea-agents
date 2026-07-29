import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import { useMonitorStore } from '../../stores/monitor/index';
import type { MonitorLogLine, MonitorTrace } from './types';
import { flowLogMatchesTrace, sortFlowLogLines, traceCorrelationFromTraceRow } from './flow';

export function useMonitorTraceFlow(detail: Ref<MonitorTrace | null>, detailOpen: Ref<boolean>) {
  const monitorStore = useMonitorStore();
  const flowLines = ref<MonitorLogLine[]>([]);
  /** Spans fetched via GetMonitorTrace detail API (monitor_trace_spans); drives waterfall/tree tabs. */
  const detailSpans = ref<unknown[]>([]);
  let flowWsSub: ReturnType<typeof monitorStore.startLogsStream> | null = null;

  const activeCorrelation = computed(() => {
    if (!detail.value) return { traceId: '', runId: '', sessionId: '' };
    return traceCorrelationFromTraceRow(detail.value);
  });

  async function loadFlowHistory() {
    const corr = activeCorrelation.value;
    if (!corr.traceId && !corr.runId && !corr.sessionId) return;
    try {
      const { items } = await monitorStore.fetchFlowLogs({
        traceId: corr.traceId || undefined,
        runId: corr.runId || undefined,
        sessionId: corr.sessionId || undefined,
        limit: 500,
      });
      const seen = new Set<string>();
      const merged = [...items, ...flowLines.value].filter((line) => {
        const key = `${line.id}-${line.time}`;
        if (seen.has(key)) return false;
        seen.add(key);
        return flowLogMatchesTrace(line, corr);
      });
      flowLines.value = sortFlowLogLines(merged);
    } catch {
      // HTTP history is best-effort; live WS still works
    }
  }

  function startFlowStream() {
    stopFlowStream();
    const corr = activeCorrelation.value;
    if (!corr.traceId && !corr.runId) return;
    const maxLines = 500;
    flowWsSub = monitorStore.startLogsStream(GLOBAL_WS_SESSION_ID, (line) => {
      if (!flowLogMatchesTrace(line, corr)) return;
      flowLines.value = [...flowLines.value, line].slice(-maxLines);
    });
  }

  function stopFlowStream() {
    flowWsSub?.close();
    flowWsSub = null;
  }

  async function openTraceDetail(row: MonitorTrace) {
    detail.value = row;
    flowLines.value = [];
    detailSpans.value = [];
    detailOpen.value = true;
    await Promise.all([loadFlowHistory(), loadSpanDetail(row.id)]);
    startFlowStream();
  }

  /** Fetch persisted spans for the waterfall/tree tabs (best-effort). */
  async function loadSpanDetail(traceId: string) {
    if (!traceId) return;
    try {
      const detailRes = await monitorStore.fetchTraceDetail(traceId);
      const spans: unknown = detailRes.spans_json ? JSON.parse(detailRes.spans_json) : [];
      detailSpans.value = Array.isArray(spans) ? spans : [];
    } catch {
      // span detail is best-effort; flow log tab still works
    }
  }

  watch(detailOpen, (open) => {
    if (!open) stopFlowStream();
  });

  onBeforeUnmount(() => stopFlowStream());

  return {
    flowLines,
    detailSpans,
    activeCorrelation,
    loadFlowHistory,
    startFlowStream,
    stopFlowStream,
    openTraceDetail,
  };
}
