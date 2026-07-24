/**
 * useFollowScroll — 状态制「关注最新消息」composable（2026-07-22 重构）。
 *
 * 取代时间制模型（10s 冷却强拽）。二态状态机，无定时器：
 *
 *   FOLLOWING ──用户滚离底部(>80px)──► UNFOLLOWED
 *       ▲                                │
 *       └────── 用户滚回底部(≤80px) ──────┘
 *
 * - FOLLOWING：contentSignature 变化 → 50ms leading+trailing rAF 节流滚底。
 * - UNFOLLOWED：停止一切自动滚动；不累计、不提示；用户滚回底部即恢复。
 * - 选区保护：滚动前检测容器内非空选区 → 转 UNFOLLOWED（保护复制操作）。
 * - programmaticScroll flag：程序滚底不触发状态切换。
 *
 * 规格：docs/superpowers/specs/2026-07-22-chat-follow-scroll-and-status-border-design.md §3.1
 */
import { nextTick, onBeforeUnmount, readonly, ref, watch, type ComputedRef, type Ref } from 'vue';

/** 距底 ≤ 该值（px）视为「在底部」。统一阈值，消除旧 20/200px 死区。 */
const NEAR_BOTTOM_THRESHOLD = 80;
/** 内容驱动滚动的节流窗口（ms）。 */
const SCROLL_THROTTLE_MS = 50;

export type FollowScrollOpts = {
  /** 滚动容器元素 ref。 */
  scrollEl: Ref<HTMLElement | null>;
  /** 内容变化签名（流式增长也必须改变签名）。 */
  contentSignature: Ref<string> | ComputedRef<string>;
  /** 是否启用跟随（如 !collapsed && status==='running'）。false→true 时滚底并进入 FOLLOWING。 */
  enabled: Ref<boolean> | ComputedRef<boolean>;
};

export function useFollowScroll(opts: FollowScrollOpts) {
  const following = ref(true);
  /** 区分程序滚动与用户滚动：程序滚动前置 true，scroll 事件消费后复位。 */
  let programmaticScroll = false;

  // 50ms leading + trailing 节流状态
  let rafId = 0;
  let lastRun = 0;
  let trailingTimer: ReturnType<typeof setTimeout> | null = null;

  function maxScrollTop(el: HTMLElement): number {
    return Math.max(0, el.scrollHeight - el.clientHeight);
  }

  function distanceFromBottom(el: HTMLElement): number {
    return maxScrollTop(el) - el.scrollTop;
  }

  /** 内容骤减（历史压缩/会话切换残留）时防 NaN / 越界。 */
  function clampScrollTop(el: HTMLElement): void {
    const max = maxScrollTop(el);
    const top = el.scrollTop;
    if (!Number.isFinite(top) || top < 0 || top > max + 2) {
      el.scrollTop = max;
    }
  }

  /**
   * 程序滚动必须瞬时：容器若带 CSS scroll-behavior:smooth（如 .chat-messages__viewport），
   * scrollTop/scrollIntoView 会变成长达 ~1.5s 的动画，期间持续派发 scroll 事件，
   * programmaticScroll 标志被首个事件消费后，后续事件被误判为「用户滚离底部」→ following=false
   * （2026-07-23 运行时验证：会话切换后偶发停在距底 669px）。
   */
  function withInstantScroll<T>(el: HTMLElement, fn: () => T): T {
    const style = el.style;
    if (!style) return fn(); // 测试桩等元素无 style 时直接执行
    const prev = style.scrollBehavior;
    style.scrollBehavior = 'auto';
    try {
      return fn();
    } finally {
      style.scrollBehavior = prev;
    }
  }

  function scrollToBottom() {
    const el = opts.scrollEl.value;
    if (!el) return;
    clampScrollTop(el);
    programmaticScroll = true;
    withInstantScroll(el, () => {
      el.scrollTop = maxScrollTop(el);
    });
  }

  /** 容器内是否存在非空选区（用户正在选择文字）。 */
  function hasSelectionInside(el: HTMLElement): boolean {
    try {
      const sel = typeof window !== 'undefined' ? window.getSelection?.() : null;
      if (!sel || sel.isCollapsed) return false;
      const node = sel.anchorNode;
      return node !== null && (node === el || el.contains(node));
    } catch {
      return false;
    }
  }

  function performScroll() {
    // 竞态守卫（2026-07-23 运行时验证发现）：scheduleScroll 注册 rAF 时 following=true，
    // 但浏览器把 scroll 事件排在同帧 rAF 回调之前派发——流式期间用户滚离的一帧内，
    // 事件先把 following 置 false，此处若不检查就会「滚回底部 → dist=0 → following 又被置 true」，
    // 用户被锁死在 FOLLOWING。执行前必须以最新状态为准。
    if (!following.value || !opts.enabled.value) return;
    lastRun = Date.now();
    scrollToBottom();
  }

  function scheduleScroll() {
    const now = Date.now();
    const elapsed = now - lastRun;
    if (elapsed >= SCROLL_THROTTLE_MS) {
      // Leading edge：距上次滚动足够久 → 下一帧立即滚
      if (rafId) return;
      rafId = requestAnimationFrame(() => {
        rafId = 0;
        performScroll();
      });
    } else {
      // Trailing：窗口内 → 剩余时间后补一次，保证最终位置正确
      if (trailingTimer) clearTimeout(trailingTimer);
      trailingTimer = setTimeout(() => {
        trailingTimer = null;
        if (rafId) return;
        rafId = requestAnimationFrame(() => {
          rafId = 0;
          performScroll();
        });
      }, SCROLL_THROTTLE_MS - elapsed);
    }
  }

  // 内容变化：仅 FOLLOWING + enabled 时滚动；容器内选区 → 转 UNFOLLOWED 保护复制。
  watch(opts.contentSignature, () => {
    if (!opts.enabled.value) return;
    if (!following.value) return;
    const el = opts.scrollEl.value;
    if (el && hasSelectionInside(el)) {
      following.value = false;
      return;
    }
    void nextTick(() => scheduleScroll());
  });

  // enabled false→true（面板展开 / agent 启动 / 会话就绪）→ 滚底 + FOLLOWING。
  // true→false → 仅停止跟随，不动滚动条。
  watch(opts.enabled, (val, prev) => {
    if (val && !prev) {
      following.value = true;
      void nextTick(() => scrollToBottom());
    }
  });

  /** 绑定容器 @scroll.passive。 */
  function onScroll(event?: Event) {
    const el = (event?.target as HTMLElement | undefined) ?? opts.scrollEl.value;
    if (!el) return;
    if (programmaticScroll) {
      programmaticScroll = false;
      clampScrollTop(el);
      return;
    }
    clampScrollTop(el);
    following.value = distanceFromBottom(el) <= NEAR_BOTTOM_THRESHOLD;
  }

  /**
   * 显式跳到最新（如「用户发新消息」场景）并恢复 FOLLOWING。
   * 传 anchorEl 时滚动到该元素顶部（block:'start'），否则滚底。
   */
  function jumpToLatest(anchorEl?: HTMLElement) {
    const el = opts.scrollEl.value;
    if (!el) return;
    following.value = true;
    if (anchorEl) {
      programmaticScroll = true;
      if (typeof anchorEl.scrollIntoView === 'function') {
        withInstantScroll(el, () => anchorEl.scrollIntoView({ block: 'start' }));
      }
      return;
    }
    scrollToBottom();
  }

  onBeforeUnmount(() => {
    if (rafId) cancelAnimationFrame(rafId);
    if (trailingTimer) clearTimeout(trailingTimer);
  });

  return {
    /** 只读跟随状态。 */
    following: readonly(following),
    onScroll,
    jumpToLatest,
    /** 程序滚底（不改 following 状态）；供会话切换对齐等多帧重试场景复用。 */
    scrollToBottom,
  };
}
