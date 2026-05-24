import { onUnmounted, ref, shallowRef } from "vue";
import { createWsTransport, type WsTransport } from "./ws-transport";
import { EnvelopeDispatcher } from "./dispatcher";
import type { Envelope, EnvelopeType } from "./envelope";
import {
  acquireGlobalWsConsumer,
  globalWsConsumerEnableLog,
  globalWsConsumerSubscribe,
  globalWsConsumerUnsubscribe,
  releaseGlobalWsConsumer,
  shouldUseGlobalWsHub,
} from "./globalWsHub";
export type {
  GraphNodeState,
  GraphExecutionState,
  GraphStreamInterrupt,
  GraphStreamExecutionSummary,
} from "../../realtime/graphState";
import type {
  GraphNodeState,
  GraphExecutionState,
  GraphStreamInterrupt,
  GraphStreamExecutionSummary,
} from "../../realtime/graphState";

export type UseEnvelopeStreamOptions = {
  sessionId: string;
  channels?: string[];
  lastEventId?: string;
  autoConnect?: boolean;
  logEnabled?: boolean;
  onConnected?: (info: { sessionId: string; lastEventId?: string }) => void;
  onDisconnected?: () => void;
  onServerShutdown?: (reason: string) => void;
  onReplayState?: (replaying: boolean, count?: number) => void;
  onReconnectFailed?: () => void;
};

export type UseEnvelopeStreamReturn = {
  connected: ReturnType<typeof ref<boolean>>;
  wsReplaying: ReturnType<typeof ref<boolean>>;
  lastEventId: ReturnType<typeof ref<string | undefined>>;
  transport: ReturnType<typeof shallowRef<WsTransport | null>>;
  dispatcher: EnvelopeDispatcher;
  connect: () => void;
  disconnect: () => void;
  onType: (type: EnvelopeType | EnvelopeType[], handler: (env: Envelope) => void) => () => void;
  onChannel: (channel: string | string[], handler: (env: Envelope) => void) => () => void;
  subscribe: (channel: string) => void;
  unsubscribe: (channel: string) => void;
  enableLog: (enabled: boolean) => void;
  cancel: () => void;
};

/** Factory for session streams; safe to call outside component `setup()` (e.g. on session select). */
export function createEnvelopeStream(opts: UseEnvelopeStreamOptions): UseEnvelopeStreamReturn {
  const connected = ref(false);
  const wsReplaying = ref(false);
  const lastEventId = ref<string | undefined>(opts.lastEventId);
  const transport = shallowRef<WsTransport | null>(null);
  const dispatcher = new EnvelopeDispatcher();

  const channels = opts.channels ?? ["chat", "system"];
  let globalHubId: string | null = null;

  function connect(): void {
    if (globalHubId) return;

    if (shouldUseGlobalWsHub(opts.sessionId, lastEventId.value)) {
      globalHubId = acquireGlobalWsConsumer({
        channels,
        logEnabled: opts.logEnabled ?? false,
        onEnvelope: (env) => dispatcher.dispatch(env),
        onConnected: () => {
          connected.value = true;
          opts.onConnected?.({ sessionId: opts.sessionId, lastEventId: lastEventId.value });
        },
        onDisconnected: () => {
          connected.value = false;
          opts.onDisconnected?.();
        },
        onServerShutdown: (reason) => {
          opts.onServerShutdown?.(reason);
        },
      });
      return;
    }

    if (transport.value?.connected) return;
    if (transport.value) {
      transport.value.connect();
      return;
    }

    const t = createWsTransport({
      sessionId: opts.sessionId,
      lastEventId: lastEventId.value,
      logEnabled: opts.logEnabled,
      onEnvelope: (env) => {
        dispatcher.dispatch(env);
      },
      onConnected: (info) => {
        connected.value = true;
        lastEventId.value = info.lastEventId;
        for (const ch of channels) {
          if (ch !== "chat" && ch !== "system") {
            t.subscribe(ch);
          }
        }
        opts.onConnected?.(info);
      },
      onDisconnected: () => {
        connected.value = false;
        opts.onDisconnected?.();
      },
      onError: () => {
        connected.value = false;
      },
      onServerShutdown: (reason) => {
        opts.onServerShutdown?.(reason);
      },
      onReplayState: (replaying, count) => {
        wsReplaying.value = replaying;
        opts.onReplayState?.(replaying, count);
      },
      onReconnectFailed: () => {
        opts.onReconnectFailed?.();
      },
    });

    transport.value = t;
    t.connect();
  }

  function disconnect(): void {
    if (globalHubId) {
      releaseGlobalWsConsumer(globalHubId);
      globalHubId = null;
      connected.value = false;
      dispatcher.clear();
      return;
    }
    transport.value?.disconnect();
    transport.value = null;
    connected.value = false;
    dispatcher.clear();
  }

  function onType(type: EnvelopeType | EnvelopeType[], handler: (env: Envelope) => void): () => void {
    return dispatcher.onType(type, handler);
  }

  function onChannel(channel: string | string[], handler: (env: Envelope) => void): () => void {
    return dispatcher.onChannel(channel, handler);
  }

  function subscribe(channel: string): void {
    if (globalHubId) {
      globalWsConsumerSubscribe(globalHubId, channel);
      return;
    }
    transport.value?.subscribe(channel);
  }

  function unsubscribe(channel: string): void {
    if (globalHubId) {
      globalWsConsumerUnsubscribe(globalHubId, channel);
      return;
    }
    transport.value?.unsubscribe(channel);
  }

  function cancel(): void {
    transport.value?.cancel();
  }

  function enableLog(enabled: boolean): void {
    if (globalHubId) {
      globalWsConsumerEnableLog(globalHubId, enabled);
      return;
    }
    transport.value?.enableLog(enabled);
  }

  if (opts.autoConnect !== false) {
    connect();
  }

  return {
    connected,
    wsReplaying,
    lastEventId,
    transport,
    dispatcher,
    connect,
    disconnect,
    onType,
    onChannel,
    subscribe,
    unsubscribe,
    enableLog,
    cancel,
  };
}

export function useEnvelopeStream(opts: UseEnvelopeStreamOptions): UseEnvelopeStreamReturn {
  const stream = createEnvelopeStream({ ...opts, autoConnect: false });
  if (opts.autoConnect !== false) {
    stream.connect();
  }
  onUnmounted(() => {
    stream.disconnect();
  });
  return stream;
}

export type ChatStreamFactoryOpts = {
  onConnected?: () => void;
  onDisconnected?: () => void;
  onServerShutdown?: (reason: string) => void;
  onReplayState?: (replaying: boolean, count?: number) => void;
  onReconnectFailed?: () => void;
};

/** Chat session WS stream; use in `setup()` via {@link useChatStream} or imperatively via this factory. */
export function createChatStream(sessionId: string, streamOpts?: ChatStreamFactoryOpts): UseEnvelopeStreamReturn {
  return createEnvelopeStream({
    sessionId,
    channels: ["chat", "system"],
    autoConnect: false,
    onConnected: () => streamOpts?.onConnected?.(),
    onDisconnected: () => streamOpts?.onDisconnected?.(),
    onServerShutdown: streamOpts?.onServerShutdown,
    onReplayState: streamOpts?.onReplayState,
    onReconnectFailed: streamOpts?.onReconnectFailed,
  });
}

export function useChatStream(sessionId: string, streamOpts?: ChatStreamFactoryOpts) {
  const stream = useEnvelopeStream({
    sessionId,
    channels: ["chat", "system"],
    onServerShutdown: streamOpts?.onServerShutdown,
    onReplayState: streamOpts?.onReplayState,
  });

  const text = ref("");
  const reasoning = ref("");
  const toolCalls = ref<Array<{ id: string; name: string; status: string }>>([]);
  const error = ref<string | null>(null);
  const done = ref(false);

  stream.onType(
    ["text_delta", "text_done"],
    (env) => {
      if (env.content?.text) {
        if (env.content.is_partial) {
          text.value += env.content.text;
        } else {
          text.value = env.content.text;
        }
      }
      if (env.content?.reasoning) {
        if (env.content.is_partial) {
          reasoning.value += env.content.reasoning;
        } else {
          reasoning.value = env.content.reasoning;
        }
      }
    }
  );

  stream.onType("tool_call", (env) => {
    if (env.tool_call) {
      const idx = toolCalls.value.findIndex((tc) => tc.id === env.tool_call!.id);
      const entry = {
        id: env.tool_call.id,
        name: env.tool_call.name,
        status: env.tool_call.status,
      };
      if (idx >= 0) {
        toolCalls.value[idx] = entry;
      } else {
        toolCalls.value.push(entry);
      }
    }
  });

  stream.onType("tool_result", (env) => {
    if (env.tool_call?.id) {
      const idx = toolCalls.value.findIndex((tc) => tc.id === env.tool_call!.id);
      if (idx >= 0) {
        toolCalls.value[idx] = { ...toolCalls.value[idx], status: env.tool_call.status };
      }
    }
  });

  stream.onType("runner_completion", () => {
    done.value = true;
  });

  stream.onType("error", (env) => {
    error.value = env.error?.message ?? "unknown error";
  });

  return {
    ...stream,
    text,
    reasoning,
    toolCalls,
    error,
    done,
    wsReplaying: stream.wsReplaying,
  };
}

export function createTeamStream(
  sessionId: string,
  streamOpts?: { onReplayState?: (replaying: boolean, count?: number) => void; onReconnectFailed?: () => void }
): UseEnvelopeStreamReturn {
  return createEnvelopeStream({
    sessionId,
    channels: ["chat", "team", "system"],
    autoConnect: false,
    onReplayState: streamOpts?.onReplayState,
    onReconnectFailed: streamOpts?.onReconnectFailed,
  });
}

export function useTeamStream(
  sessionId: string,
  streamOpts?: { onReplayState?: (replaying: boolean, count?: number) => void }
) {
  const stream = useEnvelopeStream({
    sessionId,
    channels: ["chat", "team", "system"],
    onReplayState: streamOpts?.onReplayState,
  });

  const members = ref<Map<string, { author: string; text: string }>>(new Map());

  stream.onType("member_message_start", (env) => {
    if (env.author) {
      members.value.set(env.author, { author: env.author, text: "" });
    }
  });

  stream.onType("member_delta", (env) => {
    if (env.author && env.content?.text) {
      const existing = members.value.get(env.author);
      members.value.set(env.author, {
        author: env.author,
        text: (existing?.text ?? "") + env.content.text,
      });
    }
  });

  stream.onType("member_message_done", (env) => {
    if (env.author && env.content?.text) {
      members.value.set(env.author, { author: env.author, text: env.content.text });
    }
  });

  stream.onType("transfer", (env) => {
    if (env.transfer) {
      const from = env.transfer.from_agent;
      const to = env.transfer.to_agent;
      if (from) members.value.delete(from);
      if (to) members.value.set(to, { author: to, text: "" });
    }
  });

  return {
    ...stream,
    members,
  };
}

export function useMonitorStream(sessionId: string, opts?: { global?: boolean }) {
  const effectiveSessionId = opts?.global ? "*" : sessionId;
  const stream = useEnvelopeStream({
    sessionId: effectiveSessionId,
    channels: opts?.global ? ["monitor", "chat", "team", "graph", "system"] : ["monitor", "system"],
    logEnabled: false,
  });

  const logs = ref<Array<{ level: string; message: string; timestamp: string }>>([]);
  const logEnabled = ref(false);

  function toggleLog(enabled: boolean): void {
    logEnabled.value = enabled;
    stream.enableLog(enabled);
  }

  stream.onType("log", (env) => {
    const level = (env.metadata?.level as string) ?? "INFO";
    const message = env.content?.text ?? "";
    logs.value.push({ level, message, timestamp: env.timestamp });
    if (logs.value.length > 500) {
      logs.value = logs.value.slice(-500);
    }
  });

  return {
    ...stream,
    logs,
    logEnabled,
    toggleLog,
  };
}

function parseGraphStreamSummary(raw: unknown): GraphStreamExecutionSummary | null {
  if (!raw || typeof raw !== "object") return null;
  const summary = raw as Record<string, unknown>;
  const nodes = Array.isArray(summary.nodes)
    ? summary.nodes.map((node) => {
        const n = node as Record<string, unknown>;
        return {
          nodeId: String(n.node_id ?? n.nodeId ?? ""),
          nodeType: String(n.node_type ?? n.nodeType ?? ""),
          status: String(n.status ?? ""),
          durationMs: Number(n.duration_ms ?? n.durationMs ?? 0),
          error: String(n.error ?? ""),
          stepNumber: Number(n.step_number ?? n.stepNumber ?? 0),
        };
      })
    : [];
  return {
    executionId: String(summary.execution_id ?? summary.executionId ?? ""),
    graphId: String(summary.graph_id ?? summary.graphId ?? ""),
    totalSteps: Number(summary.total_steps ?? summary.totalSteps ?? 0),
    durationMs: Number(summary.duration_ms ?? summary.durationMs ?? 0),
    finalStateKeys: Number(summary.final_state_keys ?? summary.finalStateKeys ?? 0),
    nodes,
  };
}

function parseInterruptPrompt(value: unknown): string {
  if (value == null) return "";
  if (typeof value === "string") return value.trim();
  if (typeof value === "object" && value !== null && "prompt" in value) {
    return String((value as { prompt?: unknown }).prompt ?? "").trim();
  }
  return "";
}

export function useGraphStream(sessionId: string, graphId: string, execId: string) {
  const stream = useEnvelopeStream({
    sessionId,
    channels: ["chat", "graph", "system"],
  });

  const execution = ref<GraphExecutionState>({
    executionId: execId,
    graphId,
    status: "pending",
    nodes: new Map(),
  });

  const executionSummary = ref<GraphStreamExecutionSummary | null>(null);
  const interrupt = ref<GraphStreamInterrupt | null>(null);

  const filterKey = `graph/${graphId}/${execId}`;

  stream.onChannel("graph", (env) => {
    if (env.filter_key && !env.filter_key.startsWith(filterKey)) {
      return;
    }

    switch (env.type) {
      case "graph_node_start": {
        const nodeId = env.metadata?.node_id as string;
        const nodeType = env.metadata?.node_type as string;
        const stepNumber = env.metadata?.step_number as number;
        if (nodeId) {
          const existing = execution.value.nodes.get(nodeId);
          execution.value.nodes.set(nodeId, {
            nodeId,
            nodeType: nodeType ?? existing?.nodeType ?? "function",
            status: "running",
            startTime: env.metadata?.start_time as string,
            stepNumber,
          });
          execution.value.currentNode = nodeId;
          execution.value.status = "running";
        }
        break;
      }
      case "graph_node_end": {
        const nodeId = env.metadata?.node_id as string;
        if (nodeId) {
          const existing = execution.value.nodes.get(nodeId);
          execution.value.nodes.set(nodeId, {
            nodeId,
            nodeType: (env.metadata?.node_type as string) ?? existing?.nodeType ?? "function",
            status: "completed",
            startTime: existing?.startTime,
            endTime: env.metadata?.end_time as string,
            durationNs: env.metadata?.duration_ns as number,
            stepNumber: env.metadata?.step_number as number,
          });
        }
        break;
      }
      case "graph_node_error": {
        const nodeId = env.metadata?.node_id as string;
        if (nodeId) {
          const existing = execution.value.nodes.get(nodeId);
          execution.value.nodes.set(nodeId, {
            nodeId,
            nodeType: (env.metadata?.node_type as string) ?? existing?.nodeType ?? "function",
            status: "error",
            error: env.metadata?.error as string,
            stepNumber: env.metadata?.step_number as number,
          });
          execution.value.status = "failed";
        }
        break;
      }
      case "graph_step": {
        const stepNumber = env.metadata?.step_number as number;
        if (stepNumber !== undefined) {
          execution.value.totalSteps = stepNumber;
        }
        if (env.metadata?.duration_ns) {
          execution.value.durationNs = env.metadata.duration_ns as number;
        }
        break;
      }
      case "graph_execution_done": {
        execution.value.status = "completed";
        execution.value.totalSteps = env.metadata?.total_steps as number;
        if (env.metadata?.duration_ns) {
          execution.value.durationNs = env.metadata.duration_ns as number;
        }
        executionSummary.value = parseGraphStreamSummary(env.metadata?.execution_summary);
        break;
      }
      case "checkpoint": {
        if (env.metadata?.interrupt_key) {
          execution.value.status = "waiting_human";
          const nodeId = env.metadata?.node_id as string;
          if (nodeId) {
            const existing = execution.value.nodes.get(nodeId);
            execution.value.nodes.set(nodeId, {
              nodeId,
              nodeType: (env.metadata?.node_type as string) ?? existing?.nodeType ?? "function",
              status: "interrupted",
              stepNumber: env.metadata?.step_number as number,
            });
          }
          interrupt.value = {
            nodeId: String(env.metadata?.node_id ?? ""),
            interruptKey: String(env.metadata?.interrupt_key ?? ""),
            prompt: parseInterruptPrompt(env.metadata?.interrupt_value),
            checkpointId: String(env.metadata?.checkpoint_id ?? ""),
            lineageId: String(env.metadata?.lineage_id ?? ""),
            interruptValue: env.metadata?.interrupt_value,
          };
        }
        break;
      }
    }
  });

  function clearInterrupt() {
    interrupt.value = null;
  }

  return {
    ...stream,
    execution,
    executionSummary,
    interrupt,
    clearInterrupt,
  };
}
