import { onBeforeUnmount, ref, type Ref } from 'vue';
import { storeToRefs } from 'pinia';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import { createMonitorStream } from '../../realtime/useMonitorStream';
import type { MonitorEvent } from '../../realtime/monitorEvent';
import { useMonitorStore } from '../../stores/monitor/index';
import { monitorLogLineFromFlowEvent } from './flow';
import type { MonitorLogLine, StreamState } from './types';

const MAX_LINES = 5000;

export type MonitorLogHub = {
  flowState: Ref<StreamState>;
  processState: Ref<StreamState>;
  flowPaused: Ref<boolean>;
  processPaused: Ref<boolean>;
  processEnabled: Ref<boolean>;
  flowLines: Ref<MonitorLogLine[]>;
  processLines: Ref<MonitorLogLine[]>;
  backpressureMessage: Ref<string>;
  clearBackpressure: () => void;
  connect: () => void;
  disconnect: () => void;
  setFlowPaused: (paused: boolean) => void;
  setProcessPaused: (paused: boolean) => void;
  setProcessEnabled: (enabled: boolean) => void;
  clearFlow: () => void;
  clearProcess: () => void;
};

function processLineFromEvent(ev: MonitorEvent): MonitorLogLine {
  const metaLevel = ev.metadata?.level as MonitorLogLine['level'] | undefined;
  const level = (ev.level as MonitorLogLine['level'] | undefined) ?? metaLevel ?? 'INFO';
  return {
    id: ev.id,
    time: ev.timestamp,
    level,
    message: ev.message ?? '',
    source: ev.source ?? 'monitor',
    created_at: ev.timestamp,
    kind: 'process',
  };
}

export type MonitorLogHubPausedRefs = {
  flowPaused: Ref<boolean>;
  processPaused: Ref<boolean>;
  setFlowPaused: (paused: boolean) => void;
  setProcessPaused: (paused: boolean) => void;
};

export function createMonitorLogHub(paused: MonitorLogHubPausedRefs): MonitorLogHub {
  const flowState = ref<StreamState>('connecting');
  const processState = ref<StreamState>('paused');
  const processEnabled = ref(false);
  const flowLines = ref<MonitorLogLine[]>([]);
  const processLines = ref<MonitorLogLine[]>([]);
  const backpressureMessage = ref('');
  let hasFlowLine = false;
  let hasProcessLine = false;
  let wsConnected = false;

  function refreshFlowState() {
    if (paused.flowPaused.value) {
      flowState.value = 'paused';
      return;
    }
    if (!wsConnected) {
      flowState.value = 'connecting';
      return;
    }
    flowState.value = hasFlowLine ? 'live' : 'connected';
  }

  function refreshProcessState() {
    if (!processEnabled.value) {
      processState.value = 'paused';
      return;
    }
    if (paused.processPaused.value) {
      processState.value = 'paused';
      return;
    }
    if (!wsConnected) {
      processState.value = 'connecting';
      return;
    }
    processState.value = hasProcessLine ? 'live' : 'connected';
  }

  const stream = createMonitorStream({
    sessionId: GLOBAL_WS_SESSION_ID,
    channels: ['monitor', 'system'],
    autoConnect: false,
    logEnabled: false,
    onConnected: () => {
      wsConnected = true;
      refreshFlowState();
      refreshProcessState();
      // Re-apply enableLog after connection — setProcessEnabled() may have been
      // called before the stream connected, in which case enableLog(true) was a
      // no-op. Re-sending here ensures the server actually enables log delivery.
      if (processEnabled.value) {
        stream.enableLog(true);
      }
    },
    onDisconnected: () => {
      wsConnected = false;
      if (!paused.flowPaused.value) flowState.value = 'error';
      if (processEnabled.value && !paused.processPaused.value) processState.value = 'error';
    },
    onMonitorEvent: (ev: MonitorEvent) => {
      switch (ev.type) {
        case 'flow_log': {
          if (paused.flowPaused.value) return;
          const line = monitorLogLineFromFlowEvent(ev);
          if (!line) return;
          hasFlowLine = true;
          flowState.value = 'live';
          flowLines.value = [...flowLines.value, line].slice(-MAX_LINES);
          return;
        }
        case 'log': {
          if (!processEnabled.value || paused.processPaused.value) return;
          if (ev.metadata?.flow_step || ev.metadata?.schema_version === 'flow_log/v1') {
            return;
          }
          hasProcessLine = true;
          processState.value = 'live';
          processLines.value = [...processLines.value, processLineFromEvent(ev)].slice(-MAX_LINES);
          return;
        }
        default:
          return;
      }
    },
    onBackpressure: (payload) => {
      const dropped =
        Number(payload.dropped_high ?? 0) + Number(payload.dropped_normal ?? 0) + Number(payload.dropped_low ?? 0);
      const windowSecs = Number(payload.window_seconds ?? 10);
      backpressureMessage.value = `监控流过载，最近 ${windowSecs}s 丢弃 ${dropped} 条非关键事件，可能影响实时性。`;
    },
  });

  function connect(): void {
    wsConnected = false;
    refreshFlowState();
    refreshProcessState();
    stream.connect();
  }

  function disconnect(): void {
    stream.disconnect();
    wsConnected = false;
    flowState.value = 'paused';
    processState.value = 'paused';
  }

  return {
    flowState,
    processState,
    flowPaused: paused.flowPaused,
    processPaused: paused.processPaused,
    processEnabled,
    flowLines,
    processLines,
    backpressureMessage,
    clearBackpressure: () => {
      backpressureMessage.value = '';
    },
    connect,
    disconnect,
    setFlowPaused: (p: boolean) => {
      paused.setFlowPaused(p);
      refreshFlowState();
    },
    setProcessPaused: (p: boolean) => {
      paused.setProcessPaused(p);
      refreshProcessState();
    },
    setProcessEnabled: (enabled: boolean) => {
      processEnabled.value = enabled;
      stream.enableLog(enabled);
      refreshProcessState();
    },
    clearFlow: () => {
      flowLines.value = [];
      hasFlowLine = false;
      refreshFlowState();
    },
    clearProcess: () => {
      processLines.value = [];
      hasProcessLine = false;
      refreshProcessState();
    },
  };
}

export function useMonitorLogHub(): MonitorLogHub {
  const store = useMonitorStore();
  const { flowPaused, processPaused } = storeToRefs(store);
  const hub = createMonitorLogHub({
    flowPaused,
    processPaused,
    setFlowPaused: store.setFlowPaused,
    setProcessPaused: store.setProcessPaused,
  });
  onBeforeUnmount(() => hub.disconnect());
  return hub;
}
