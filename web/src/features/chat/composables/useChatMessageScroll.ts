import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type ComputedRef, type Ref } from 'vue';
import type { Message } from '../types';
import type { Task } from '../v2Types';

/** Distance from bottom (px) to consider "at the bottom" for immediate recovery. */
const NEAR_BOTTOM_THRESHOLD = 20;
/** Distance from bottom (px) to show the "scroll to bottom" button. */
const SCROLL_BTN_THRESHOLD = 200;
/** Idle period after the last user scroll before auto-scroll resumes (ms). */
const RECOVERY_MS = 10_000;

export type ChatMessageScrollOpts = {
  sessionKey: Ref<string> | ComputedRef<string>;
  messages: Ref<Message[]>;
  messagesScrollEl: Ref<HTMLElement | null>;
  /** B-04 / Activity-First: tasks driving v2 SessionPanel rendering.
   *  Watching its length ensures new tasks trigger auto-scroll just like
   *  new messages. */
  tasks?: Ref<Task[]> | ComputedRef<Task[]>;
};

export function useChatMessageScroll(opts: ChatMessageScrollOpts) {
  const showScrollBtn = ref(false);
  /**
   * Time-based auto-scroll model (matching useActivityAutoScroll):
   * - `recoveryTimer === null`  → auto-scroll is ACTIVE (content changes scroll to bottom)
   * - `recoveryTimer !== null`  → user scrolled recently, auto-scroll PAUSED (10s cooldown)
   * - Each user scroll away from bottom resets the 10s timer
   * - When the timer fires → scroll to bottom & resume auto-scroll
   * - User scrolls near bottom → clear cooldown (resume immediately)
   */
  let recoveryTimer: ReturnType<typeof setTimeout> | null = null;
  /** Distinguishes programmatic scrollToBottom() from user-initiated scrolls. */
  let programmaticScroll = false;

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

  function isNearBottom(el: HTMLElement): boolean {
    return distanceFromBottom(el) <= NEAR_BOTTOM_THRESHOLD;
  }

  function activeScrollEl(): HTMLElement | null {
    return opts.messagesScrollEl.value;
  }

  function clampScrollTop(el: HTMLElement, preferBottom: boolean): void {
    const max = maxScrollTop(el);
    const top = el.scrollTop;
    if (!Number.isFinite(top) || top < 0 || top > max + 2) {
      el.scrollTop = preferBottom ? max : 0;
    }
  }

  function clearRecoveryTimer() {
    if (recoveryTimer) {
      clearTimeout(recoveryTimer);
      recoveryTimer = null;
    }
  }

  /** Whether auto-scroll is currently active (no pending user cooldown). */
  function isAutoScrollActive(): boolean {
    return recoveryTimer === null;
  }

  function onMessagesScroll(event?: Event) {
    const el = (event?.target as HTMLElement | undefined) ?? activeScrollEl();
    if (!el) return;

    // Ignore programmatic scrolls (from scrollToBottom).
    if (programmaticScroll) {
      programmaticScroll = false;
      clampScrollTop(el, true);
      return;
    }

    clampScrollTop(el, true);
    const dist = distanceFromBottom(el);
    showScrollBtn.value = dist > SCROLL_BTN_THRESHOLD;

    // User scrolled near the bottom — treat as "still following", resume immediately.
    if (isNearBottom(el)) {
      clearRecoveryTimer();
      return;
    }

    // User scrolled away from bottom — (re)start the 10s recovery timer.
    clearRecoveryTimer();
    recoveryTimer = setTimeout(() => {
      recoveryTimer = null;
      // 10s of no user scrolling — resume auto-scroll immediately.
      void nextTick(() => {
        void scrollToBottom(false);
      });
    }, RECOVERY_MS);
  }

  async function scrollToBottom(smooth = false) {
    const el = opts.messagesScrollEl.value;
    if (!el) return;
    clampScrollTop(el, true);
    const top = maxScrollTop(el);
    programmaticScroll = true;
    el.scrollTo({ top, behavior: smooth ? 'smooth' : 'auto' });
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
      programmaticScroll = true;
      el.scrollTop = top;
      if (preferBottom && el.clientHeight > 0 && isNearBottom(el)) {
        showScrollBtn.value = false;
        return;
      }
      if (!preferBottom && el.scrollTop <= 1) {
        return;
      }
    }
  }

  // Session switched: clear any user cooldown and scroll to bottom.
  watch(
    () => opts.sessionKey.value,
    () => {
      clearRecoveryTimer();
      showScrollBtn.value = false;
      void alignMessageScroll(true);
    },
  );

  watch(
    () => opts.messages.value.length,
    (len, prev) => {
      if (len === 0) return;
      if (prev === 0) {
        // First message — clear cooldown and scroll to bottom.
        clearRecoveryTimer();
        void alignMessageScroll(true);
        return;
      }
      // Auto-scroll only if no pending user cooldown.
      if (!isAutoScrollActive()) return;
      void scrollToBottom(false);
    },
  );

  // B-04 / Activity-First: auto-scroll when the tasks array grows.
  // New tasks render in v2 SessionPanel, but messages.length may stay
  // unchanged, so the messages watcher above would not trigger. This
  // watcher closes the gap.
  watch(
    () => opts.tasks?.value.length ?? 0,
    (len, prev) => {
      if (len === 0) return;
      if (prev === 0) {
        clearRecoveryTimer();
        void alignMessageScroll(true);
        return;
      }
      if (!isAutoScrollActive()) return;
      scheduleScrollToBottom();
    },
  );

  // P1#4: auto-scroll when a task transitions to a terminal status. The tasks
  // array length may not change (a running task transitions to completed), so
  // the length watcher alone misses it. We watch a signature of the latest
  // terminal task and throttle the scroll with requestAnimationFrame.
  const lastFinalReplySignature = computed(() => {
    const tasks = opts.tasks?.value ?? [];
    for (let i = tasks.length - 1; i >= 0; i--) {
      const t = tasks[i];
      if (t.Status === 'completed' || t.Status === 'failed' || t.Status === 'cancelled') {
        return `${t.ID}:${t.Status}:${t.CompletedAt ?? ''}`;
      }
    }
    return '';
  });
  watch(lastFinalReplySignature, (sig, prev) => {
    if (!sig || sig === prev) return;
    if (!isAutoScrollActive()) return;
    scheduleScrollToBottom();
  });

  onMounted(() => {
    if (opts.messages.value.length > 0) {
      clearRecoveryTimer();
      void alignMessageScroll(true);
    }
  });

  // Leading-edge throttle with trailing call: ensures the first delta in each
  // 50ms window triggers a scroll, and the last delta always gets a trailing
  // scroll so the view doesn't get stuck mid-stream.
  let scrollStickRaf = 0;
  let scrollStickLastRun = 0;
  let scrollStickTrailingTimer: ReturnType<typeof setTimeout> | null = null;
  const SCROLL_STICK_THROTTLE_MS = 50;
  function scheduleScrollToBottom() {
    if (scrollStickRaf) return;
    scrollStickRaf = requestAnimationFrame(() => {
      scrollStickRaf = 0;
      scrollStickLastRun = Date.now();
      void scrollToBottom(false);
    });
  }
  watch(
    () => opts.messages.value[opts.messages.value.length - 1]?.content_markdown ?? '',
    () => {
      if (!isAutoScrollActive()) return;
      const now = Date.now();
      const elapsed = now - scrollStickLastRun;
      if (elapsed >= SCROLL_STICK_THROTTLE_MS) {
        // Leading edge: enough time has passed — scroll immediately
        scrollStickLastRun = now;
        scheduleScrollToBottom();
      } else {
        // Within throttle window — schedule a trailing scroll after the
        // remaining wait so the final position is always correct.
        if (scrollStickTrailingTimer) clearTimeout(scrollStickTrailingTimer);
        scrollStickTrailingTimer = setTimeout(() => {
          scrollStickTrailingTimer = null;
          if (!isAutoScrollActive()) return;
          scheduleScrollToBottom();
        }, SCROLL_STICK_THROTTLE_MS - elapsed);
      }
    },
  );

  onBeforeUnmount(() => {
    clearRecoveryTimer();
    if (scrollStickRaf) cancelAnimationFrame(scrollStickRaf);
    if (scrollStickTrailingTimer) clearTimeout(scrollStickTrailingTimer);
    if (highlightTimer) clearTimeout(highlightTimer);
  });

  async function scrollToTurnId(turnId: string, smooth = true) {
    const id = turnId.trim();
    if (!id) return;
    await nextTick();
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

    // Handle collapse/expand hints
    const collapseHint = target.closest<HTMLElement>('.code-block__collapsed-hint');
    if (collapseHint) {
      event.preventDefault();
      const block = collapseHint.closest<HTMLElement>('.code-block');
      if (!block) return;
      const body = block.querySelector<HTMLElement>('.code-block__body');
      const collapseHintEl = block.querySelector<HTMLElement>('.code-block__collapse-hint');
      if (body) body.style.display = '';
      if (collapseHint) collapseHint.style.display = 'none';
      if (collapseHintEl) collapseHintEl.style.display = '';
      block.classList.remove('code-block--collapsed');
      return;
    }

    const expandHint = target.closest<HTMLElement>('.code-block__collapse-hint');
    if (expandHint) {
      event.preventDefault();
      const block = expandHint.closest<HTMLElement>('.code-block');
      if (!block) return;
      const body = block.querySelector<HTMLElement>('.code-block__body');
      const collapsedHint = block.querySelector<HTMLElement>('.code-block__collapsed-hint');
      if (body) body.style.display = 'none';
      if (expandHint) expandHint.style.display = 'none';
      if (collapsedHint) collapsedHint.style.display = '';
      block.classList.add('code-block--collapsed');
      return;
    }

    // Handle copy button
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
