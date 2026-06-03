import { computed, ref, watch, type ComputedRef, type Ref } from 'vue';
import type { Message } from '../types';
import { reasoningMarkdown } from '../streamContentPatch';

export type ReasoningSidebarState = {
  open: Ref<boolean>;
  pinnedMessageId: Ref<string | null>;
  activeReasoning: ComputedRef<{
    messageId: string;
    reasoning: string;
    streaming: boolean;
  } | null>;
  toggle: () => void;
  pinMessage: (messageId: string) => void;
  unpin: () => void;
};

export function useReasoningSidebar(deps: {
  messages: ComputedRef<Message[]>;
  sessionId: ComputedRef<string | undefined>;
}): ReasoningSidebarState {
  const open = ref(false);
  const pinnedMessageId = ref<string | null>(null);

  const streamingMessage = computed(() => {
    const msgs = deps.messages.value;
    return msgs.find(
      (m) =>
        m.role === 'assistant' &&
        (m.status === 'streaming' || m.status === 'tool_running') &&
        (reasoningMarkdown(m)?.trim() ?? '').length > 0,
    );
  });

  const pinnedMessage = computed(() => {
    const pid = pinnedMessageId.value;
    if (!pid) return null;
    return deps.messages.value.find((m) => m.id === pid) ?? null;
  });

  const activeReasoning = computed(() => {
    const stream = streamingMessage.value;
    if (stream) {
      const r = reasoningMarkdown(stream) ?? '';
      if (r.trim()) {
        return {
          messageId: stream.id,
          reasoning: r,
          streaming: true,
        };
      }
    }
    const pinned = pinnedMessage.value;
    if (pinned) {
      const r = reasoningMarkdown(pinned) ?? '';
      if (r.trim()) {
        return {
          messageId: pinned.id,
          reasoning: r,
          streaming: false,
        };
      }
    }
    if (!stream) {
      const msgs = deps.messages.value;
      for (let i = msgs.length - 1; i >= 0; i--) {
        const m = msgs[i];
        if (m.role === 'assistant' && m.status === 'ok') {
          const r = reasoningMarkdown(m) ?? '';
          if (r.trim()) {
            return {
              messageId: m.id,
              reasoning: r,
              streaming: false,
            };
          }
        }
      }
    }
    return null;
  });

  watch(deps.sessionId, () => {
    pinnedMessageId.value = null;
  });

  function toggle() {
    open.value = !open.value;
  }

  function pinMessage(messageId: string) {
    pinnedMessageId.value = messageId;
    if (!open.value) open.value = true;
  }

  function unpin() {
    pinnedMessageId.value = null;
  }

  return { open, pinnedMessageId, activeReasoning, toggle, pinMessage, unpin };
}
