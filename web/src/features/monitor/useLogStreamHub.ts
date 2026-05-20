import { onBeforeUnmount, ref } from "vue";
import { GLOBAL_WS_SESSION_ID } from "../../config/runtime";
import { createEnvelopeStream } from "../chat/useEnvelopeStream";
import type { Envelope } from "../chat/envelope";
import { monitorLogLineFromFlowEnvelope } from "./flow";
import type { MonitorLogLine, StreamState } from "./types";

const MAX_LINES = 5000;

export type MonitorLogHub = {
  flowState: ReturnType<typeof ref<StreamState>>;
  processState: ReturnType<typeof ref<StreamState>>;
  flowPaused: ReturnType<typeof ref<boolean>>;
  processPaused: ReturnType<typeof ref<boolean>>;
  processEnabled: ReturnType<typeof ref<boolean>>;
  flowLines: ReturnType<typeof ref<MonitorLogLine[]>>;
  processLines: ReturnType<typeof ref<MonitorLogLine[]>>;
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

export function createMonitorLogHub(): MonitorLogHub {
  const flowState = ref<StreamState>("connecting");
  const processState = ref<StreamState>("paused");
  const flowPaused = ref(false);
  const processPaused = ref(true);
  const processEnabled = ref(false);
  const flowLines = ref<MonitorLogLine[]>([]);
  const processLines = ref<MonitorLogLine[]>([]);
  let hasFlowLine = false;
  let hasProcessLine = false;
  let wsConnected = false;

  function refreshFlowState() {
    if (flowPaused.value) {
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
    if (processPaused.value) {
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
      if (!flowPaused.value) flowState.value = "error";
      if (processEnabled.value && !processPaused.value) processState.value = "error";
    },
  });

  stream.onType("flow_log", (env: Envelope) => {
    if (flowPaused.value) return;
    const line = monitorLogLineFromFlowEnvelope(env);
    if (!line) return;
    hasFlowLine = true;
    flowState.value = "live";
    flowLines.value = [...flowLines.value, line].slice(-MAX_LINES);
  });

  stream.onType("log", (env: Envelope) => {
    if (!processEnabled.value || processPaused.value) return;
    if (env.metadata?.flow_step || env.metadata?.schema_version === "flow_log/v1") {
      return;
    }
    hasProcessLine = true;
    processState.value = "live";
    processLines.value = [...processLines.value, processLineFromEnvelope(env)].slice(-MAX_LINES);
  });

  stream.onType("error", () => {
    if (!flowPaused.value) flowState.value = "error";
    if (processEnabled.value && !processPaused.value) processState.value = "error";
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

  function setFlowPaused(paused: boolean): void {
    flowPaused.value = paused;
    refreshFlowState();
  }

  function setProcessPaused(paused: boolean): void {
    processPaused.value = paused;
    refreshProcessState();
  }

  function setProcessEnabled(enabled: boolean): void {
    processEnabled.value = enabled;
    stream.enableLog(enabled);
    refreshProcessState();
  }

  return {
    flowState,
    processState,
    flowPaused,
    processPaused,
    processEnabled,
    flowLines,
    processLines,
    connect,
    disconnect,
    setFlowPaused,
    setProcessPaused,
    setProcessEnabled,
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
  const hub = createMonitorLogHub();
  onBeforeUnmount(() => hub.disconnect());
  return hub;
}
