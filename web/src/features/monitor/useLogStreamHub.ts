import { onBeforeUnmount, ref, type Ref } from 'vue';
import { storeToRefs } from 'pinia';
import { getBackendOrigin, GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import { i18n } from '../../i18n';
import { createMonitorStream } from '../../realtime/useMonitorStream';
import type { MonitorEvent } from '../../realtime/monitorEvent';
import { useMonitorStore } from '../../stores/monitor/index';
import { monitorLogLineFromFlowEvent } from './flow';
import type { MonitorLogLine, StreamState } from './types';
import { normalizeLogLevel } from './utils';

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
  /** 连接异常时的精确原因提示（如 429 连接数已满 / 401 未登录）；无则空串。 */
  errorHint: Ref<string>;
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
  const metaLevel = ev.metadata?.level as string | undefined;
  const level = normalizeLogLevel((ev.level as string | undefined) ?? metaLevel);
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
  const errorHint = ref('');
  let hasFlowLine = false;
  let hasProcessLine = false;
  let wsConnected = false;
  let errorProbed = false;

  // 浏览器 WS API 拿不到握手失败的 HTTP 状态码；进入 error 时对 /v1/ws 做一次
  // 普通 GET 探测（不会建立 WS 连接），从状态码还原精确原因（429 连接数已满 /
  // 401 登录过期）。每个异常 episode 只探测一次，成功连接后复位。
  async function probeErrorHint(): Promise<void> {
    try {
      const resp = await fetch(`${getBackendOrigin()}/v1/ws?session_id=*`, { credentials: 'include' });
      if (resp.status === 429) {
        errorHint.value = i18n.global.t('monitorPage.logs.connLimitHint');
      } else if (resp.status === 401 || resp.status === 403) {
        errorHint.value = i18n.global.t('monitorPage.logs.authExpiredHint');
      }
    } catch {
      // 网络层失败——保持笼统的「连接异常」即可
    }
  }

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
      errorProbed = false;
      errorHint.value = '';
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
      if (!errorProbed) {
        errorProbed = true;
        void probeErrorHint();
      }
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
    errorHint,
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
