import { nextTick, onBeforeUnmount, onMounted, ref, watch, type ComputedRef, type Ref } from 'vue';
import type { QVirtualScroll } from 'quasar';
import { isActivityMessage } from '../mergeSessionMessages';
import { groupMessagesByTurn, lastAssistantTurnBlockIndex, type TurnBlockGroup } from '../groupMessagesByTurn';
import type { Message } from '../types';

const SCROLL_BOTTOM_THRESHOLD = 80;

export type ChatMessageScrollOpts = {
  sessionKey: Ref<string> | ComputedRef<string>;
  messages: Ref<Message[]>;
  useTurnBlockMode: ComputedRef<boolean>;
  turnBlocks: ComputedRef<TurnBlockGroup[]>;
  useVirtualMessageList: ComputedRef<boolean>;
  timelineItemsLength: ComputedRef<number>;
  virtualScrollRef: Ref<QVirtualScroll | null>;
  messagesScrollEl: Ref<HTMLElement | null>;
};

export function useChatMessageScroll(opts: ChatMessageScrollOpts) {
  const showScrollBtn = ref(false);
  const stickToBottom = ref(true);
  const highlightedTurnId = ref<string | undefined>(undefined);
  let highlightTimer: ReturnType<typeof setTimeout> | null = null;

  function flashTurnHighlight(turnId: string) {
    highlightedTurnId.value = turnId;
    if (highlightTimer) clearTimeout(highlightTimer);
    highlightTimer = setTimeout(() => {
      highlightedTurnId.value = undefined;
      highlightTimer = null;
    }, 2000);
  }

  function maxScrollTop(el: HTMLElement): number {
    return Math.max(0, el.scrollHeight - el.clientHeight);
  }

  function distanceFromBottom(el: HTMLElement): number {
    return maxScrollTop(el) - el.scrollTop;
  }

  function activeScrollEl(): HTMLElement | null {
    if (opts.useVirtualMessageList.value && opts.virtualScrollRef.value) {
      return opts.virtualScrollRef.value.$el as HTMLElement;
    }
    return opts.messagesScrollEl.value;
  }

  function clampScrollTop(el: HTMLElement, preferBottom: boolean): void {
    const max = maxScrollTop(el);
    const top = el.scrollTop;
    if (!Number.isFinite(top) || top < 0 || top > max + 2) {
      el.scrollTop = preferBottom ? max : 0;
    }
  }

  function onMessagesScroll(event?: Event) {
    const el = (event?.target as HTMLElement | undefined) ?? activeScrollEl();
    if (!el) return;
    clampScrollTop(el, stickToBottom.value);
    const dist = distanceFromBottom(el);
    showScrollBtn.value = dist > 200;
    stickToBottom.value = dist <= SCROLL_BOTTOM_THRESHOLD;
  }

  function lastDialogueIndex(): number {
    if (opts.useTurnBlockMode.value) {
      return lastAssistantTurnBlockIndex(opts.turnBlocks.value);
    }
    for (let i = opts.messages.value.length - 1; i >= 0; i--) {
      const m = opts.messages.value[i]!;
      if (m.role === 'user' && (m.content_markdown ?? '').trim()) return i;
      if (m.role === 'assistant' && !isActivityMessage(m) && (m.content_markdown ?? '').trim()) {
        return i;
      }
    }
    return Math.max(0, opts.messages.value.length - 1);
  }

  async function scrollToLatestDialogue(smooth = false) {
    const idx = lastDialogueIndex();
    if (opts.useVirtualMessageList.value && opts.virtualScrollRef.value) {
      for (let attempt = 0; attempt < 4; attempt++) {
        await nextTick();
        if (opts.virtualScrollRef.value) {
          opts.virtualScrollRef.value.scrollTo(idx, smooth ? 'start' : 'start-force');
          stickToBottom.value = true;
          showScrollBtn.value = false;
          return;
        }
        await new Promise((resolve) => requestAnimationFrame(resolve));
      }
      return;
    }
    const el = opts.messagesScrollEl.value;
    if (el) {
      const rows = el.querySelectorAll<HTMLElement>(opts.useTurnBlockMode.value ? '.turn-block' : '.chat-q-message');
      const target = rows[idx];
      if (target) {
        target.scrollIntoView({ block: 'start', behavior: smooth ? 'smooth' : 'auto' });
        stickToBottom.value = true;
        showScrollBtn.value = false;
        return;
      }
    }
    await scrollToBottom(smooth);
  }

  async function scrollToBottom(smooth = false) {
    if (opts.useVirtualMessageList.value && opts.timelineItemsLength.value > 0) {
      for (let attempt = 0; attempt < 3; attempt++) {
        await nextTick();
        if (opts.virtualScrollRef.value) {
          opts.virtualScrollRef.value.scrollTo(opts.timelineItemsLength.value - 1, smooth ? 'start' : 'start-force');
          stickToBottom.value = true;
          showScrollBtn.value = false;
          return;
        }
        await new Promise((resolve) => requestAnimationFrame(resolve));
      }
      return;
    }
    const el = opts.messagesScrollEl.value;
    if (!el) return;
    clampScrollTop(el, true);
    const top = maxScrollTop(el);
    el.scrollTo({ top, behavior: smooth ? 'smooth' : 'auto' });
    stickToBottom.value = true;
    showScrollBtn.value = false;
  }

  async function alignMessageScroll(preferBottom: boolean) {
    for (let attempt = 0; attempt < 4; attempt++) {
      await nextTick();
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      const el = activeScrollEl();
      if (!el) continue;
      clampScrollTop(el, preferBottom);
      const top = preferBottom ? maxScrollTop(el) : 0;
      el.scrollTop = top;
      if (preferBottom && el.clientHeight > 0 && distanceFromBottom(el) <= SCROLL_BOTTOM_THRESHOLD) {
        stickToBottom.value = true;
        showScrollBtn.value = false;
        return;
      }
      if (!preferBottom && el.scrollTop <= 1) {
        return;
      }
    }
  }

  watch(
    () => opts.sessionKey.value,
    () => {
      stickToBottom.value = true;
      showScrollBtn.value = false;
      void alignMessageScroll(true);
    },
  );

  watch(
    () => opts.messages.value.length,
    (len, prev) => {
      if (len === 0) return;
      if (prev === 0) {
        stickToBottom.value = true;
        void alignMessageScroll(true);
        return;
      }
      if (!stickToBottom.value) return;
      void scrollToBottom(false);
    },
  );

  onMounted(() => {
    if (opts.messages.value.length > 0) {
      stickToBottom.value = true;
      void alignMessageScroll(true);
    }
  });

  watch(opts.useVirtualMessageList, (enabled) => {
    if (enabled && opts.messages.value.length > 0 && stickToBottom.value) {
      void scrollToLatestDialogue(false);
    }
  });

  let scrollStickRaf = 0;
  let scrollStickThrottle = 0;
  watch(
    () => opts.messages.value[opts.messages.value.length - 1]?.content_markdown ?? '',
    () => {
      if (!stickToBottom.value) return;
      if (scrollStickRaf) return;
      const now = Date.now();
      if (now - scrollStickThrottle < 50) {
        scrollStickRaf = requestAnimationFrame(() => {
          scrollStickRaf = 0;
          scrollStickThrottle = Date.now();
          void scrollToBottom(false);
        });
        return;
      }
      scrollStickThrottle = now;
      scrollStickRaf = requestAnimationFrame(() => {
        scrollStickRaf = 0;
        void scrollToBottom(false);
      });
    },
  );

  onBeforeUnmount(() => {
    if (scrollStickRaf) cancelAnimationFrame(scrollStickRaf);
    if (highlightTimer) clearTimeout(highlightTimer);
  });

  async function scrollToTurnId(turnId: string, smooth = true) {
    const id = turnId.trim();
    if (!id || !opts.useTurnBlockMode.value) return;
    await nextTick();
    if (opts.useVirtualMessageList.value && opts.virtualScrollRef.value) {
      const idx = opts.turnBlocks.value.findIndex((b) => b.turnId === id || b.user?.id === id);
      if (idx >= 0) {
        opts.virtualScrollRef.value.scrollTo(idx, smooth ? 'start' : 'start-force');
        flashTurnHighlight(id);
      }
      return;
    }
    const el = opts.messagesScrollEl.value;
    if (!el) return;
    const target = el.querySelector<HTMLElement>(`[data-turn-id="${CSS.escape(id)}"]`);
    if (target) {
      target.scrollIntoView({ block: 'start', behavior: smooth ? 'smooth' : 'auto' });
      flashTurnHighlight(id);
    }
  }

  return {
    showScrollBtn,
    highlightedTurnId,
    onMessagesScroll,
    scrollToBottom,
    scrollToTurnId,
  };
}

export function useChatCodeCopy() {
  let copyResetTimer: ReturnType<typeof setTimeout> | null = null;

  function handleMessagesClick(event: MouseEvent) {
    const target = event.target as HTMLElement | null;
    if (!target) return;
    const btn = target.closest<HTMLButtonElement>('.code-block__copy');
    if (!btn) return;
    event.preventDefault();
    const block = btn.closest<HTMLElement>('.code-block');
    const code = block?.querySelector<HTMLElement>('pre code')?.innerText ?? '';
    if (!code) return;
    const apply = () => {
      btn.classList.add('is-copied');
      const textEl = btn.querySelector<HTMLElement>('.code-block__copy-text');
      const original = textEl?.textContent ?? '复制';
      if (textEl) textEl.textContent = '已复制';
      if (copyResetTimer) clearTimeout(copyResetTimer);
      copyResetTimer = setTimeout(() => {
        btn.classList.remove('is-copied');
        if (textEl) textEl.textContent = original;
      }, 1400);
    };
    if (navigator.clipboard?.writeText) {
      void navigator.clipboard
        .writeText(code)
        .then(apply)
        .catch(() => fallbackCopy(code, apply));
    } else {
      fallbackCopy(code, apply);
    }
  }

  function fallbackCopy(text: string, onSuccess: () => void) {
    try {
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      document.body.removeChild(ta);
      onSuccess();
    } catch {
      /* swallow */
    }
  }

  onBeforeUnmount(() => {
    if (copyResetTimer) clearTimeout(copyResetTimer);
  });

  return { handleMessagesClick };
}
