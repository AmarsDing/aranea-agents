import { computed, ref, type ComputedRef, type Ref } from "vue";
import type { QVirtualScroll } from "quasar";
import type { Message } from "./types";

const HEADER_VIEWPORT_OFFSET = 72;

/** Normalize user message body for header / scroll markers. */
export function userPromptText(message: Message): string {
  const raw = (message.content_markdown ?? "").trim();
  if (!raw) return "";
  return raw.replace(/\s+/g, " ");
}

/** @deprecated use userPromptText */
export const userPromptPreview = userPromptText;

function lastUserPrompt(messages: Message[]): string {
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i]!;
    if (m.role !== "user") continue;
    const text = userPromptText(m);
    if (text) return text;
  }
  return "";
}

function resolveScrollRoot(
  useVirtual: boolean,
  virtualScrollRef: Ref<QVirtualScroll | null>,
  messagesScrollEl: Ref<HTMLElement | null>,
): HTMLElement | null {
  if (useVirtual && virtualScrollRef.value) {
    return virtualScrollRef.value.$el as HTMLElement;
  }
  return messagesScrollEl.value;
}

/** Tracks which **user** turn is at the top of the viewport for the header prompt bar. */
export function useChatScrollTitle(opts: {
  sessionTitle: Ref<string> | ComputedRef<string>;
  messages: Ref<Message[]>;
  messagesScrollEl: Ref<HTMLElement | null>;
  virtualScrollRef: Ref<QVirtualScroll | null>;
  useVirtualMessageList: Ref<boolean>;
}) {
  const activeUserPrompt = ref("");

  const headerUserPrompt = computed(() => {
    const prompt = activeUserPrompt.value.trim();
    if (prompt) return prompt;
    if (opts.messages.value.length > 0) {
      return lastUserPrompt(opts.messages.value);
    }
    return "";
  });

  const promptKey = computed(() => headerUserPrompt.value || "__empty__");

  function refreshActivePrompt() {
    const root = resolveScrollRoot(
      opts.useVirtualMessageList.value,
      opts.virtualScrollRef,
      opts.messagesScrollEl,
    );
    if (!root) {
      activeUserPrompt.value = lastUserPrompt(opts.messages.value);
      return;
    }

    const markers = root.querySelectorAll<HTMLElement>("[data-chat-user-prompt]");
    if (!markers.length) {
      activeUserPrompt.value = "";
      return;
    }

    const anchor = root.getBoundingClientRect().top + HEADER_VIEWPORT_OFFSET;
    let bestTop = Number.POSITIVE_INFINITY;
    let bestText = "";

    markers.forEach((el) => {
      const rect = el.getBoundingClientRect();
      if (rect.bottom < anchor - 8) return;
      const text = (el.dataset.chatUserPrompt ?? "").trim();
      if (!text) return;
      if (rect.top < bestTop) {
        bestTop = rect.top;
        bestText = text;
      }
    });

    activeUserPrompt.value = bestText;
  }

  function resetToLatestOrSession() {
    activeUserPrompt.value = "";
  }

  return {
    activeUserPrompt,
    headerUserPrompt,
    promptKey,
    refreshActivePrompt,
    resetToLatestOrSession,
  };
}
