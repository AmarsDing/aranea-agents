import { onBeforeUnmount, ref, type Ref } from "vue";
import { storeToRefs } from "pinia";
import { GLOBAL_WS_SESSION_ID } from "../../config/runtime";
import { createEnvelopeStream } from "../../realtime/useEnvelopeStream";
import type { Envelope } from "../../realtime/envelope";
import { useMonitorStore } from "../../stores/monitor/index";
import { monitorLogLineFromFlowEnvelope } from "./flow";
import type { MonitorLogLine, StreamState } from "./types";

const MAX_LINES = 5000;

export type MonitorLogHub = {
  flowState: Ref<StreamState>;
  processState: Ref<StreamState>;
  flowPaused: Ref<boolean>;
  processPaused: Ref<boolean>;
  processEnabled: Ref<boolean>;
  flowLines: Ref<MonitorLogLine[]>;
  processLines: Ref<MonitorLogLine[]>;
  connect: () => void;
  disconnect: () => void;
  setFlowPaused: (paused: boolean) => void;
  setProcessPaused: (paused: boolean) => void;
  setProcessEnabled: (enabled: boolean) => void;
  clearFlow: () => void;
  clearProcess: () => void;
};

function processLineFromEnvelope(env: Envelope): MonitorLogLine {
  const level = (env.metadata?.level as MonitorLogLine["level"]) ?? "INFO";
  return {
    id: env.id,
    time: env.timestamp,
    level,
    message: env.content?.text ?? "",
    source: env.author ?? "monitor",
    created_at: env.timestamp,
    kind: "process",
  };
}

export type MonitorLogHubPausedRefs = {
  flowPaused: Ref<boolean>;
  processPaused: Ref<boolean>;
  setFlowPaused: (paused: boolean) => void;
  setProcessPaused: (paused: boolean) => void;
};

export function createMonitorLogHub(paused: MonitorLogHubPausedRefs): MonitorLogHub {
  const flowState = ref<StreamState>("connecting");
  const processState = ref<StreamState>("paused");
  const processEnabled = ref(false);
  const flowLines = ref<MonitorLogLine[]>([]);
  const processLines = ref<MonitorLogLine[]>([]);
  let hasFlowLine = false;
  let hasProcessLine = false;
  let wsConnected = false;

  function refreshFlowState() {
    if (paused.flowPaused.value) {
      flowState.value = "paused";
      return;
    }
    if (!wsConnected) {
      flowState.value = "connecting";
      return;
    }
    flowState.value = hasFlowLine ? "live" : "connected";
  }

  function refreshProcessState() {
    if (!processEnabled.value) {
      processState.value = "paused";
      return;
    }
    if (paused.processPaused.value) {
      processState.value = "paused";
      return;
    }
    if (!wsConnected) {
      processState.value = "connecting";
      return;
    }
    processState.value = hasProcessLine ? "live" : "connected";
  }

  const stream = createEnvelopeStream({
    sessionId: GLOBAL_WS_SESSION_ID,
    channels: ["monitor", "system"],
    autoConnect: false,
    logEnabled: false,
    onConnected: () => {
      wsConnected = true;
      refreshFlowState();
      refreshProcessState();
    },
    onDisconnected: () => {
      wsConnected = false;
      if (!paused.flowPaused.value) flowState.value = "error";
      if (processEnabled.value && !paused.processPaused.value) processState.value = "error";
    },
  });

  stream.onType("flow_log", (env: Envelope) => {
    if (paused.flowPaused.value) return;
    const line = monitorLogLineFromFlowEnvelope(env);
    if (!line) return;
    hasFlowLine = true;
    flowState.value = "live";
    flowLines.value = [...flowLines.value, line].slice(-MAX_LINES);
  });

  stream.onType("log", (env: Envelope) => {
    if (!processEnabled.value || paused.processPaused.value) return;
    if (env.metadata?.flow_step || env.metadata?.schema_version === "flow_log/v1") {
      return;
    }
    hasProcessLine = true;
    processState.value = "live";
    processLines.value = [...processLines.value, processLineFromEnvelope(env)].slice(-MAX_LINES);
  });

  stream.onType("error", () => {
    if (!paused.flowPaused.value) flowState.value = "error";
    if (processEnabled.value && !paused.processPaused.value) processState.value = "error";
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
    flowState.value = "paused";
    processState.value = "paused";
  }

  return {
    flowState,
    processState,
    flowPaused: paused.flowPaused,
    processPaused: paused.processPaused,
    processEnabled,
    flowLines,
    processLines,
    connect,
    disconnect,
    setFlowPaused: paused.setFlowPaused,
    setProcessPaused: paused.setProcessPaused,
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
