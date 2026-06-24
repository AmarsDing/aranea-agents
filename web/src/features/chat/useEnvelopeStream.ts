/**
 * Chat-specific envelope stream helpers + backward-compatible re-exports.
 *
 * - createChatStream / useChatStream: Chat session WS stream helpers
 * - createTeamStream / useTeamStream: Team session WS stream helpers
 * - useMonitorStream: Monitor/log stream helper
 * - Re-exports from realtime/ for backward compatibility
 *
 * New code that only needs the generic stream should import from
 * "realtime/useEnvelopeStream" directly.
 */

export { createEnvelopeStream, useEnvelopeStream } from '../../realtime/useEnvelopeStream';

export type { UseEnvelopeStreamOptions, UseEnvelopeStreamReturn } from '../../realtime/useEnvelopeStream';

export type {
  GraphNodeState,
  GraphExecutionState,
  GraphStreamInterrupt,
  GraphStreamExecutionSummary,
} from '../../realtime/graphState';

import { ref } from 'vue';
import { createEnvelopeStream, useEnvelopeStream } from '../../realtime/useEnvelopeStream';
import { CHAT_ENVELOPE_LOG_LOCAL_MAX } from '../constants/queryLimits';
import type { UseEnvelopeStreamReturn } from '../../realtime/useEnvelopeStream';

export type ChatStreamFactoryOpts = {
  lastEventId?: string;
  onConnected?: () => void;
  onDisconnected?: () => void;
  onServerShutdown?: (reason: string) => void;
  onReplayState?: (replaying: boolean, count?: number) => void;
};

/** Chat session WS stream; use in `setup()` via {@link useChatStream} or imperatively via this factory. */
export function createChatStream(sessionId: string, streamOpts?: ChatStreamFactoryOpts): UseEnvelopeStreamReturn {
  return createEnvelopeStream({
    sessionId,
    channels: ['chat', 'system'],
    autoConnect: false,
    lastEventId: streamOpts?.lastEventId,
    onConnected: () => streamOpts?.onConnected?.(),
    onDisconnected: () => streamOpts?.onDisconnected?.(),
    onServerShutdown: streamOpts?.onServerShutdown,
    onReplayState: streamOpts?.onReplayState,
  });
}

export function useChatStream(sessionId: string, streamOpts?: ChatStreamFactoryOpts) {
  const stream = useEnvelopeStream({
    sessionId,
    channels: ['chat', 'system'],
    onServerShutdown: streamOpts?.onServerShutdown,
    onReplayState: streamOpts?.onReplayState,
  });

  const text = ref('');
  const reasoning = ref('');
  const toolCalls = ref<Array<{ id: string; name: string; status: string }>>([]);
  const error = ref<string | null>(null);
  const done = ref(false);

  stream.onType(['text_delta', 'text_done'], (env) => {
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
  });

  stream.onType('tool_call', (env) => {
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

  stream.onType('tool_result', (env) => {
    if (env.tool_call?.id) {
      const idx = toolCalls.value.findIndex((tc) => tc.id === env.tool_call!.id);
      if (idx >= 0) {
        toolCalls.value[idx] = { ...toolCalls.value[idx], status: env.tool_call.status };
      }
    }
  });

  stream.onType('runner_completion', () => {
    done.value = true;
  });

  stream.onType('error', (env) => {
    error.value = env.error?.message ?? 'unknown error';
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
  streamOpts?: {
    onReplayState?: (replaying: boolean, count?: number) => void;
    onConnected?: () => void;
    onServerShutdown?: (reason: string) => void;
  },
): UseEnvelopeStreamReturn {
  return createEnvelopeStream({
    sessionId,
    channels: ['chat', 'team', 'system'],
    autoConnect: false,
    onReplayState: streamOpts?.onReplayState,
    onConnected: () => streamOpts?.onConnected?.(),
    onServerShutdown: streamOpts?.onServerShutdown,
  });
}

export function useTeamStream(
  sessionId: string,
  streamOpts?: { onReplayState?: (replaying: boolean, count?: number) => void },
) {
  const stream = useEnvelopeStream({
    sessionId,
    channels: ['chat', 'team', 'system'],
    onReplayState: streamOpts?.onReplayState,
  });

  const members = ref<Map<string, { author: string; text: string }>>(new Map());

  stream.onType('member_message_start', (env) => {
    if (env.author) {
      members.value.set(env.author, { author: env.author, text: '' });
    }
  });

  stream.onType('member_delta', (env) => {
    if (env.author && env.content?.text) {
      const existing = members.value.get(env.author);
      members.value.set(env.author, {
        author: env.author,
        text: (existing?.text ?? '') + env.content.text,
      });
    }
  });

  stream.onType('member_message_done', (env) => {
    if (env.author && env.content?.text) {
      members.value.set(env.author, { author: env.author, text: env.content.text });
    }
  });

  stream.onType('transfer', (env) => {
    if (env.transfer) {
      const from = env.transfer.from_agent;
      const to = env.transfer.to_agent;
      if (from) members.value.delete(from);
      if (to) members.value.set(to, { author: to, text: '' });
    }
  });

  return {
    ...stream,
    members,
  };
}

export function useMonitorStream(sessionId: string, opts?: { global?: boolean }) {
  const effectiveSessionId = opts?.global ? '*' : sessionId;
  const stream = useEnvelopeStream({
    sessionId: effectiveSessionId,
    channels: opts?.global ? ['monitor', 'chat', 'team', 'graph', 'system'] : ['monitor', 'system'],
    logEnabled: false,
  });

  const logs = ref<Array<{ level: string; message: string; timestamp: string }>>([]);
  const logEnabled = ref(false);

  function toggleLog(enabled: boolean): void {
    logEnabled.value = enabled;
    stream.enableLog(enabled);
  }

  stream.onType('log', (env) => {
    const level = (env.metadata?.level as string) ?? 'INFO';
    const message = env.content?.text ?? '';
    logs.value.push({ level, message, timestamp: env.timestamp });
    if (logs.value.length > CHAT_ENVELOPE_LOG_LOCAL_MAX) {
      logs.value = logs.value.slice(-CHAT_ENVELOPE_LOG_LOCAL_MAX);
    }
  });

  return {
    ...stream,
    logs,
    logEnabled,
    toggleLog,
  };
}

export { useGraphStream } from '../graph/runtime/useGraphStream';
