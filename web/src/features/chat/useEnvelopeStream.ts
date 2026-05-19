import { onUnmounted, ref, shallowRef } from "vue";
import { createWsTransport, type WsTransport } from "./ws-transport";
import { EnvelopeDispatcher } from "./dispatcher";
import type { Envelope, EnvelopeType } from "./envelope";

export type UseEnvelopeStreamOptions = {
  sessionId: string;
  channels?: string[];
  lastEventId?: string;
  autoConnect?: boolean;
  logEnabled?: boolean;
  onServerShutdown?: (reason: string) => void;
  onReplayState?: (replaying: boolean, count?: number) => void;
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

export function useEnvelopeStream(opts: UseEnvelopeStreamOptions): UseEnvelopeStreamReturn {
  const connected = ref(false);
  const wsReplaying = ref(false);
  const lastEventId = ref<string | undefined>(opts.lastEventId);
  const transport = shallowRef<WsTransport | null>(null);
  const dispatcher = new EnvelopeDispatcher();

  const channels = opts.channels ?? ["chat", "system"];

  function connect(): void {
    if (transport.value?.connected) return;

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
      },
      onDisconnected: () => {
        connected.value = false;
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
    });

    transport.value = t;
    t.connect();
  }

  function disconnect(): void {
    transport.value?.disconnect();
    transport.value = null;
    connected.value = false;
  }

  function onType(type: EnvelopeType | EnvelopeType[], handler: (env: Envelope) => void): () => void {
    return dispatcher.onType(type, handler);
  }

  function onChannel(channel: string | string[], handler: (env: Envelope) => void): () => void {
    return dispatcher.onChannel(channel, handler);
  }

  function subscribe(channel: string): void {
    transport.value?.subscribe(channel);
  }

  function unsubscribe(channel: string): void {
    transport.value?.unsubscribe(channel);
  }

  function cancel(): void {
    transport.value?.cancel();
  }

  function enableLog(enabled: boolean): void {
    transport.value?.enableLog(enabled);
  }

  if (opts.autoConnect !== false) {
    connect();
  }

  onUnmounted(() => {
    disconnect();
    dispatcher.clear();
  });

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

export function useChatStream(
  sessionId: string,
  streamOpts?: { onServerShutdown?: (reason: string) => void; onReplayState?: (replaying: boolean, count?: number) => void }
) {
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

export type GraphNodeState = {
  nodeId: string;
  nodeType: string;
  status: "pending" | "running" | "completed" | "error" | "interrupted";
  startTime?: string;
  endTime?: string;
  durationNs?: number;
  error?: string;
  stepNumber?: number;
};

export type GraphExecutionState = {
  executionId: string;
  graphId: string;
  status: "pending" | "running" | "completed" | "failed" | "cancelled" | "waiting_human";
  currentNode?: string;
  totalSteps?: number;
  durationNs?: number;
  nodes: Map<string, GraphNodeState>;
};

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
        }
        break;
      }
    }
  });

  return {
    ...stream,
    execution,
  };
}
