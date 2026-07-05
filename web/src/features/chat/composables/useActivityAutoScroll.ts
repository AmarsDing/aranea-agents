/**
 * useActivityAutoScroll — time-based auto-scroll composable for agent activity panels.
 *
 * Design rationale (Task 5, 2026-07-05):
 *   The existing `useChatMessageScroll` uses a threshold model (stick-to-bottom
 *   if within 80px). That model is not suitable for the agent activity panel
 *   because the user requirement is explicitly time-based: "当人为触控时，听从
 *   用户的操作，当用户不操作了(10s)再自动刷新到最后".
 *
 *   Model:
 *   - `recoveryTimer === null`  → auto-scroll is ACTIVE (content changes scroll to bottom)
 *   - `recoveryTimer !== null`  → user scrolled recently, auto-scroll PAUSED (10s cooldown)
 *   - Each user scroll resets the 10s timer
 *   - When the timer fires → scroll to bottom & resume auto-scroll
 *   - When panel expands → clear cooldown & scroll to bottom immediately
 *
 *   A `programmaticScroll` flag distinguishes our `scrollToBottom()` calls from
 *   genuine user scrolls, so programmatic scrolls don't trigger the cooldown.
 */

import { nextTick, onBeforeUnmount, watch, type ComputedRef, type Ref } from 'vue';

export type ActivityAutoScrollOpts = {
  /** The scrollable element ref (e.g., the .member-activities container). */
  scrollEl: Ref<HTMLElement | null>;
  /**
   * A signature that changes whenever the scrollable content updates
   * (e.g., `${steps.length}:${lastStepId}`). Used to trigger auto-scroll
   * on content changes.
   */
  contentSignature: ComputedRef<string> | Ref<string>;
  /**
   * Whether auto-scroll should be active at all. Typically:
   *   `!collapsed && status === 'running'`
   * When this becomes true (panel expanded / agent started), we scroll to
   * bottom immediately and clear any user cooldown.
   */
  enabled: ComputedRef<boolean> | Ref<boolean>;
};

/** Idle period after the last user scroll before auto-scroll resumes (ms). */
const RECOVERY_MS = 10_000;
/** Tolerance for considering the viewport "at the bottom" (px). */
const NEAR_BOTTOM_THRESHOLD = 20;

export function useActivityAutoScroll(opts: ActivityAutoScrollOpts) {
  let recoveryTimer: ReturnType<typeof setTimeout> | null = null;
  // Distinguishes programmatic scrollToBottom() from user-initiated scrolls.
  // Set to true right before we assign scrollTop; the subsequent scroll event
  // sees the flag, resets it, and skips the cooldown logic.
  let programmaticScroll = false;

  function clearRecoveryTimer() {
    if (recoveryTimer) {
      clearTimeout(recoveryTimer);
      recoveryTimer = null;
    }
  }

  function isNearBottom(): boolean {
    const el = opts.scrollEl.value;
    if (!el) return true;
    return el.scrollHeight - el.scrollTop - el.clientHeight <= NEAR_BOTTOM_THRESHOLD;
  }

  function scrollToBottom() {
    const el = opts.scrollEl.value;
    if (!el) return;
    programmaticScroll = true;
    el.scrollTop = el.scrollHeight;
  }

  /**
   * Scroll event handler — attach to the container's @scroll.
   *
   * - Programmatic scrolls (from scrollToBottom) are ignored.
   * - User scrolls that land near the bottom are treated as "still following"
   *   and do NOT start a cooldown (keeps auto-scroll active).
   * - User scrolls away from the bottom start/refresh the 10s cooldown.
   */
  function onScroll() {
    if (programmaticScroll) {
      programmaticScroll = false;
      return;
    }
    // User scrolled to near the bottom — treat as "still following", no cooldown.
    if (isNearBottom()) {
      clearRecoveryTimer();
      return;
    }
    // User scrolled away from bottom — (re)start the 10s recovery timer.
    clearRecoveryTimer();
    recoveryTimer = setTimeout(() => {
      recoveryTimer = null;
      // 10s of no user scrolling — resume auto-scroll immediately.
      if (opts.enabled.value) {
        void nextTick(() => scrollToBottom());
      }
    }, RECOVERY_MS);
  }

  // Content changed: auto-scroll only if no pending user cooldown.
  watch(
    opts.contentSignature,
    () => {
      if (!opts.enabled.value) return;
      if (recoveryTimer) return; // user is in control during cooldown
      void nextTick(() => scrollToBottom());
    },
  );

  // Panel expanded (or agent started running): scroll to bottom & clear cooldown.
  watch(
    opts.enabled,
    (val) => {
      if (val) {
        clearRecoveryTimer();
        void nextTick(() => scrollToBottom());
      }
    },
  );

  onBeforeUnmount(() => {
    clearRecoveryTimer();
  });

  return { onScroll, scrollToBottom };
}
