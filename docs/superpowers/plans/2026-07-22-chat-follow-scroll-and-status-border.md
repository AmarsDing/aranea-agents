# 聊天「关注最新消息」机制重构实施计划：状态制跟随 + 左边线状态样式

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用「状态制跟随（FOLLOWING/UNFOLLOWED，无定时器）」替换聊天两层滚动体系的时间制模型，并用「左边线 + 状态色」替换团队区域的三层边框盒套娃。

**Architecture:** 新增统一 composable `useFollowScroll`（状态机 + 50ms 节流 + 选区保护），外层 `useChatMessageScroll` 重写为其薄封装（末端签名 + 新 Task 锚点），内层 `MemberSessionPanel` 直接换用；样式层仅改 scoped sass，DOM 结构不变。

**Tech Stack:** Vue 3 Composition API / TypeScript / Pinia（经 `useActivityQueries` 访问）/ Vitest + jsdom / sass(scoped)。

**规格文档:** `docs/superpowers/specs/2026-07-22-chat-follow-scroll-and-status-border-design.md`（已确认）

**项目纪律（覆盖 skill 默认行为）：**
- **不自动 git commit**——项目规则要求仅在用户明确要求时提交。每个 Task 末尾以「验证」收尾而非 commit。
- Windows PowerShell 环境：测试命令使用 `pnpm --dir web test ...`；若 C 盘空间不足导致构建失败，先 `$env:GOTMPDIR='D:\tmp'`（仅 Go 构建需要，本计划纯前端）。

---

## 文件结构

| 文件 | 职责 | 动作 |
|------|------|------|
| `web/src/features/chat/composables/useFollowScroll.ts` | 状态制跟随核心（状态机/节流/选区保护） | 新增 |
| `web/src/features/chat/composables/__tests__/useFollowScroll.spec.ts` | 核心单测 | 新增 |
| `web/src/features/chat/composables/useChatMessageScroll.ts` | 外层薄封装（签名驱动 + 新 Task 锚点 + turn 定位高亮 + 代码复制） | 重写前半部分（`useChatCodeCopy` 不动） |
| `web/src/features/chat/composables/__tests__/useChatMessageScroll.spec.ts` | 外层封装单测 | 重写 |
| `web/src/features/chat/composables/useActivityQueries.ts` | 增加 `teamRuns()` 只读访问器 | 修改 |
| `web/src/components/chat/ChatMessagePanel.vue` | 组装末端签名/lastTaskId 传入；移除 showScrollBtn 接线 | 修改 |
| `web/src/components/chat/ChatMessageList.vue` | 移除 ↓ 回底按钮/prop/emit | 修改 |
| `web/src/components/chat/v2/MemberSessionPanel.vue` | 换用 useFollowScroll + 左边线样式 | 修改 |
| `web/src/components/chat/v2/TeamRunCard.vue` | 左边线状态样式（含 running 脉冲） | 修改 |
| `web/src/components/chat/v2/TeamStagePanel.vue` | 去边框/背景 | 修改 |
| `web/src/features/chat/composables/useActivityAutoScroll.ts` | 旧时间制 composable | 删除 |
| `web/src/i18n/locales/zh-CN.ts` / `en-US.ts` | 删除 `chat.scrollToLatest` | 修改 |
| `web/src/css/theme/_chat-message-panel.sass` | 删除 `.chat-scroll-bottom` / `.chat-scroll-fade` 样式 | 修改 |
| `docs/development/1-chat.design.md` | 同步 B.2.2 / B.4.5 / 新增 B.4.6（DOC-SYNC） | 修改 |

---

## Task 1: useFollowScroll 核心 composable（TDD）

**Files:**
- Create: `web/src/features/chat/composables/useFollowScroll.ts`
- Test: `web/src/features/chat/composables/__tests__/useFollowScroll.spec.ts`

**设计要点（与规格 §3.1 一致）：**
- 二态状态机：`following: ref(true)`；用户滚离底部（>80px）→ false；滚回（≤80px）→ true。无任何定时器。
- `programmaticScroll` flag：程序滚底产生的 scroll 事件不触发状态切换。
- 内容签名变化 + FOLLOWING + enabled → 50ms leading+trailing rAF 节流滚底。
- 选区保护：滚动前检测容器内非空选区 → 转 UNFOLLOWED。
- `enabled` false→true → 滚底 + FOLLOWING；true→false → 仅停止跟随，不动滚动条。
- 返回 `{ following, onScroll, jumpToLatest, scrollToBottom }`（`scrollToBottom` 供外层会话对齐复用；`jumpToLatest(anchorEl?)` 支持锚点元素 `scrollIntoView({block:'start'})`）。
- jsdom 兼容：滚底用 `el.scrollTop = maxScrollTop(el)` 赋值（不用 `el.scrollTo()`）；`scrollIntoView` 用 `typeof === 'function'` 守卫。

- [ ] **Step 1: 写失败测试**

创建 `web/src/features/chat/composables/__tests__/useFollowScroll.spec.ts`：

```ts
// web/src/features/chat/composables/__tests__/useFollowScroll.spec.ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ref, nextTick } from 'vue';
import { useFollowScroll } from '../useFollowScroll';

/** 1000px 内容 / 400px 视口的假滚动容器。distanceFromBottom = 600 - scrollTop。 */
function fakeEl(overrides: Record<string, unknown> = {}): HTMLElement {
  return {
    scrollTop: 0,
    scrollHeight: 1000,
    clientHeight: 400,
    contains: () => false,
    ...overrides,
  } as unknown as HTMLElement;
}

function setup(elOverrides: Record<string, unknown> = {}) {
  const el = fakeEl(elOverrides);
  const scrollEl = ref<HTMLElement | null>(el);
  const contentSignature = ref('a');
  const enabled = ref(true);
  const api = useFollowScroll({ scrollEl, contentSignature, enabled });
  return { el, scrollEl, contentSignature, enabled, ...api };
}

describe('useFollowScroll', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => setTimeout(() => cb(0), 0));
    vi.stubGlobal('cancelAnimationFrame', (id: number) => clearTimeout(id));
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('初始为 FOLLOWING；签名变化时（节流后）滚底', async () => {
    const { el, contentSignature, following } = setup();
    expect(following.value).toBe(true);
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(60);
    expect(el.scrollTop).toBe(600);
  });

  it('用户滚离底部 >80px → UNFOLLOWED，签名变化不再滚动', async () => {
    const { el, contentSignature, onScroll, following } = setup();
    el.scrollTop = 100; // dist = 500 > 80
    onScroll();
    expect(following.value).toBe(false);
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(60);
    expect(el.scrollTop).toBe(100);
  });

  it('UNFOLLOWED 中滚回 ≤80px → 恢复 FOLLOWING，随后签名变化恢复滚底', async () => {
    const { el, contentSignature, onScroll, following } = setup();
    el.scrollTop = 100;
    onScroll();
    expect(following.value).toBe(false);
    el.scrollTop = 560; // dist = 40 ≤ 80
    onScroll();
    expect(following.value).toBe(true);
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(60);
    expect(el.scrollTop).toBe(600);
  });

  it('程序滚底（scrollToBottom）产生的事件不切换状态', async () => {
    const { el, onScroll, scrollToBottom, following } = setup();
    el.scrollTop = 100;
    onScroll();
    expect(following.value).toBe(false);
    scrollToBottom();
    expect(el.scrollTop).toBe(600);
    onScroll(); // 消费 programmatic flag，不改 following
    expect(following.value).toBe(false); // 用户未主动滚回，仍 UNFOLLOWED
  });

  it('容器内非空选区 → 转 UNFOLLOWED 保护复制', async () => {
    const node = {} as Node;
    const { el, contentSignature, following } = setup({ contains: (n: Node) => n === node });
    vi.spyOn(window, 'getSelection').mockReturnValue({ isCollapsed: false, anchorNode: node } as Selection);
    contentSignature.value = 'b';
    await nextTick();
    expect(following.value).toBe(false);
    vi.restoreAllMocks();
  });

  it('容器外选区不影响跟随', async () => {
    const { el, contentSignature, following } = setup({ contains: () => false });
    vi.spyOn(window, 'getSelection').mockReturnValue({ isCollapsed: false, anchorNode: {} as Node } as Selection);
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(60);
    expect(following.value).toBe(true);
    expect(el.scrollTop).toBe(600);
    vi.restoreAllMocks();
  });

  it('enabled false→true：滚底并进入 FOLLOWING；true→false：不动滚动条', async () => {
    const { el, enabled, contentSignature, onScroll, following } = setup();
    // 先进入 UNFOLLOWED 并关闭 enabled
    el.scrollTop = 100;
    onScroll();
    enabled.value = false;
    await nextTick();
    el.scrollTop = 50;
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(60);
    expect(el.scrollTop).toBe(50); // disabled：不滚动
    // 重新启用 → 滚底 + FOLLOWING
    enabled.value = true;
    await nextTick();
    expect(el.scrollTop).toBe(600);
    expect(following.value).toBe(true);
  });

  it('jumpToLatest() 滚底并恢复 FOLLOWING', async () => {
    const { el, onScroll, jumpToLatest, following } = setup();
    el.scrollTop = 100;
    onScroll();
    expect(following.value).toBe(false);
    jumpToLatest();
    expect(el.scrollTop).toBe(600);
    expect(following.value).toBe(true);
  });

  it('jumpToLatest(anchorEl) 调用 scrollIntoView 并恢复 FOLLOWING', async () => {
    const { onScroll, jumpToLatest, following } = setup();
    onScroll();
    // 仍处于 FOLLOWING（初始 true，dist=600? 不——初始 scrollTop=0 → dist=600>80）
    expect(following.value).toBe(false);
    const anchor = { scrollIntoView: vi.fn() } as unknown as HTMLElement;
    jumpToLatest(anchor);
    expect(anchor.scrollIntoView).toHaveBeenCalledWith({ block: 'start' });
    expect(following.value).toBe(true);
  });

  it('scrollEl 为 null 时全部 no-op', async () => {
    const scrollEl = ref<HTMLElement | null>(null);
    const contentSignature = ref('a');
    const { onScroll, jumpToLatest, scrollToBottom } = useFollowScroll({
      scrollEl,
      contentSignature,
      enabled: ref(true),
    });
    expect(() => {
      onScroll();
      jumpToLatest();
      scrollToBottom();
    }).not.toThrow();
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(60);
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- run src/features/chat/composables/__tests__/useFollowScroll.spec.ts`
Expected: FAIL — `Failed to resolve import "../useFollowScroll"`（模块不存在）。

- [ ] **Step 3: 实现 useFollowScroll**

创建 `web/src/features/chat/composables/useFollowScroll.ts`：

```ts
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

  function scrollToBottom() {
    const el = opts.scrollEl.value;
    if (!el) return;
    clampScrollTop(el);
    programmaticScroll = true;
    el.scrollTop = maxScrollTop(el);
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
        if (!following.value || !opts.enabled.value) return;
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
        anchorEl.scrollIntoView({ block: 'start' });
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
```

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm --dir web test -- run src/features/chat/composables/__tests__/useFollowScroll.spec.ts`
Expected: PASS（10 个用例全过）。

- [ ] **Step 5: 验证（不提交）**

Run: `pnpm --dir web lint --quiet src/features/chat/composables/useFollowScroll.ts`
Expected: 无错误输出。

---

## Task 2: 重写 useChatMessageScroll 为薄封装（TDD）

**Files:**
- Modify: `web/src/features/chat/composables/useChatMessageScroll.ts`（重写 `useChatMessageScroll` 部分；`useChatCodeCopy` 保持原样不动）
- Test: `web/src/features/chat/composables/__tests__/useChatMessageScroll.spec.ts`（重写）

**Breaking changes（下游在 Task 4/5 同步）：**
- opts 移除 `tasks`，新增 `contentSignature`（调用方组装的末端签名）与 `lastTaskId`（新 Task 锚点）。
- 返回值移除 `showScrollBtn`；保留 `highlightedTurnId / onMessagesScroll / scrollToBottom / scrollToTurnId`。
- `alignMessageScroll` 简化：删除 `preferBottom` 参数（现状全部调用点都传 `true`，顶部对齐路径是死代码）。

- [ ] **Step 1: 重写失败测试**

全量替换 `web/src/features/chat/composables/__tests__/useChatMessageScroll.spec.ts`：

```ts
// web/src/features/chat/composables/__tests__/useChatMessageScroll.spec.ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ref, nextTick } from 'vue';
import { useChatMessageScroll } from '../useChatMessageScroll';

function fakeEl(overrides: Record<string, unknown> = {}): HTMLElement {
  return {
    scrollTop: 0,
    scrollHeight: 1000,
    clientHeight: 400,
    contains: () => false,
    querySelector: () => null,
    ...overrides,
  } as unknown as HTMLElement;
}

function setup(elOverrides: Record<string, unknown> = {}) {
  const el = fakeEl(elOverrides);
  const messagesScrollEl = ref<HTMLElement | null>(el);
  const sessionKey = ref('s1');
  const messages = ref<any[]>([]);
  const contentSignature = ref('a');
  const lastTaskId = ref('t1');
  const api = useChatMessageScroll({ sessionKey, messages, messagesScrollEl, contentSignature, lastTaskId });
  return { el, sessionKey, messages, contentSignature, lastTaskId, ...api };
}

describe('useChatMessageScroll（状态制重构）', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => setTimeout(() => cb(0), 0));
    vi.stubGlobal('cancelAnimationFrame', (id: number) => clearTimeout(id));
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('不再返回 showScrollBtn；保留 turn 高亮/定位 API', () => {
    const api = setup();
    expect('showScrollBtn' in api).toBe(false);
    expect(typeof api.scrollToBottom).toBe('function');
    expect(typeof api.scrollToTurnId).toBe('function');
    expect(api.highlightedTurnId.value).toBeUndefined();
  });

  it('末端签名变化（跟随中）→ 节流滚底', async () => {
    const { el, contentSignature } = setup({ scrollTop: 600 });
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(60);
    expect(el.scrollTop).toBe(600);
  });

  it('用户滚离后签名变化不滚动（无定时器强拽）', async () => {
    const { el, contentSignature, onMessagesScroll } = setup();
    el.scrollTop = 100;
    onMessagesScroll();
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(el.scrollTop).toBe(100); // 10s 后也不会被拽回
  });

  it('新 Task 出现 → scrollIntoView 锚定到 [data-task-id] 元素顶部', async () => {
    const anchor = { scrollIntoView: vi.fn() } as unknown as HTMLElement;
    const { lastTaskId } = setup({ querySelector: (sel: string) => (sel.includes('t2') ? anchor : null) });
    lastTaskId.value = 't2';
    await nextTick();
    await nextTick();
    expect(anchor.scrollIntoView).toHaveBeenCalledWith({ block: 'start' });
  });

  it('scrollToTurnId 定位并高亮 2s', async () => {
    const target = { scrollIntoView: vi.fn() } as unknown as HTMLElement;
    const { highlightedTurnId, scrollToTurnId } = setup({
      querySelector: () => target,
    });
    await scrollToTurnId('turn-9');
    expect(target.scrollIntoView).toHaveBeenCalled();
    expect(highlightedTurnId.value).toBe('turn-9');
    await vi.advanceTimersByTimeAsync(2100);
    expect(highlightedTurnId.value).toBeUndefined();
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- run src/features/chat/composables/__tests__/useChatMessageScroll.spec.ts`
Expected: FAIL — 旧实现仍返回 `showScrollBtn`，且 opts 类型不含 `contentSignature`（TS 报错或断言失败）。

- [ ] **Step 3: 重写实现**

将 `web/src/features/chat/composables/useChatMessageScroll.ts` 中 **第 1 行至第 282 行**（即文件头部到 `useChatMessageScroll` 函数结束、`useChatCodeCopy` 之前）整体替换为：

```ts
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type ComputedRef, type Ref } from 'vue';
import type { Message } from '../types';
import { useFollowScroll } from './useFollowScroll';

export type ChatMessageScrollOpts = {
  sessionKey: Ref<string> | ComputedRef<string>;
  messages: Ref<Message[]>;
  messagesScrollEl: Ref<HTMLElement | null>;
  /**
   * 活动树末端签名（调用方组装，O(1)）：
   * messages.length : lastMessage.content.length : tasks.length : 末端turn的steps.length
   *   : 末端step.ID : 末端step.Status : 末端step.Content.length : teamStages.size : teamRuns.size
   */
  contentSignature: Ref<string> | ComputedRef<string>;
  /** session 活动树最新 task ID；新 Task 出现时锚定到其 UserMessage 顶部（G5）。 */
  lastTaskId: Ref<string> | ComputedRef<string>;
};

export function useChatMessageScroll(opts: ChatMessageScrollOpts) {
  const follow = useFollowScroll({
    scrollEl: opts.messagesScrollEl,
    contentSignature: opts.contentSignature,
    enabled: computed(() => opts.sessionKey.value.trim().length > 0),
  });

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

  /**
   * 会话切换 / 挂载：多帧重试滚底 + 恢复 FOLLOWING。
   * 内容可能分帧渲染（WS replay / 懒加载），最多重试 4 帧。
   */
  async function alignMessageScroll() {
    for (let attempt = 0; attempt < 4; attempt++) {
      await nextTick();
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      const el = opts.messagesScrollEl.value;
      if (!el) continue;
      follow.jumpToLatest();
      if (el.clientHeight > 0 && el.scrollHeight - el.scrollTop - el.clientHeight <= 80) {
        return;
      }
    }
  }

  // 会话切换：滚底 + FOLLOWING。
  watch(
    () => opts.sessionKey.value,
    () => {
      void alignMessageScroll();
    },
  );

  // G5 新 Task 锚点：用户发新消息 → 滚到 TaskCard（UserMessage）顶部 + FOLLOWING。
  watch(
    () => opts.lastTaskId.value,
    async (id, prev) => {
      if (!id || id === prev) return;
      await nextTick();
      const el = opts.messagesScrollEl.value;
      if (!el) return;
      const target = el.querySelector<HTMLElement>(`[data-task-id="${CSS.escape(id)}"]`);
      if (target) follow.jumpToLatest(target);
    },
  );

  onMounted(() => {
    void alignMessageScroll();
  });

  onBeforeUnmount(() => {
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
    highlightedTurnId,
    onMessagesScroll: follow.onScroll,
    scrollToBottom: follow.scrollToBottom,
    scrollToTurnId,
  };
}
```

**注意**：文件第 284 行起的 `useChatCodeCopy` 函数（含 `handleMessagesClick` / `fallbackCopy`）原样保留，不做任何改动。

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm --dir web test -- run src/features/chat/composables/__tests__/useChatMessageScroll.spec.ts`
Expected: PASS（5 个用例全过）。

- [ ] **Step 5: 验证编译（下游尚未接线，预期报错清单明确）**

Run: `pnpm --dir web exec vue-tsc --noEmit -p tsconfig.json 2>&1 | Select-String -Pattern 'ChatMessagePanel|ChatMessageList|MemberSessionPanel'`
Expected: 报出 `ChatMessagePanel.vue`（`showScrollBtn` 不存在 / `tasks` 未知 opt）与 `MemberSessionPanel.vue`（`useActivityAutoScroll` 仍在但被弃用不影响）相关错误——这些在 Task 4/5/6 修复。若其他文件报错则需排查。

---

## Task 3: MemberSessionPanel 换用 useFollowScroll

**Files:**
- Modify: `web/src/components/chat/v2/MemberSessionPanel.vue:38,91-96,210-224`
- Test: `web/src/components/chat/v2/__tests__/MemberSessionPanel.spec.ts`（无需改动——现有用例不 mock composable，scrollEl 为 null 时全部 no-op）

> 样式改造在 Task 6 统一做（一次只改一个明确问题——R5）。本任务仅换 composable。

- [ ] **Step 1: 替换 import 与调用**

[MemberSessionPanel.vue](file:///f:/aranea-agents/web/src/components/chat/v2/MemberSessionPanel.vue#L95) 第 95 行：

```ts
import { useActivityAutoScroll } from '../../../features/chat/composables/useActivityAutoScroll';
```

替换为：

```ts
import { useFollowScroll } from '../../../features/chat/composables/useFollowScroll';
```

第 210-224 行（自动滚动段）：

```ts
// 自动滚动：running + 展开时实时滚到底；用户滚动后 10s 恢复
const activitiesRef = ref<HTMLElement | null>(null);
const autoScrollEnabled = computed(() => !collapsed.value && props.memberSession.Status === 'running');
// 内容签名：steps 数量 + 最后一步 ID + 内容长度（检测流式增长）
const contentSignature = computed(() => {
  const steps = memberSteps.value;
  if (steps.length === 0) return '0:';
  const last = steps[steps.length - 1];
  return `${steps.length}:${last.ID}:${last.Content?.length ?? 0}`;
});
const { onScroll } = useActivityAutoScroll({
  scrollEl: activitiesRef,
  contentSignature,
  enabled: autoScrollEnabled,
});
```

替换为（注释同步为状态制语义）：

```ts
// 自动滚动（状态制）：running + 展开时跟随滚底；用户滚离后永不自动滚动，滚回底部即恢复
const activitiesRef = ref<HTMLElement | null>(null);
const autoScrollEnabled = computed(() => !collapsed.value && props.memberSession.Status === 'running');
// 内容签名：steps 数量 + 最后一步 ID + 内容长度（检测流式增长）
const contentSignature = computed(() => {
  const steps = memberSteps.value;
  if (steps.length === 0) return '0:';
  const last = steps[steps.length - 1];
  return `${steps.length}:${last.ID}:${last.Content?.length ?? 0}`;
});
const { onScroll } = useFollowScroll({
  scrollEl: activitiesRef,
  contentSignature,
  enabled: autoScrollEnabled,
});
```

模板第 38 行 scroll 绑定加 `.passive`（与外层视口一致）：

```vue
<div v-if="memberSteps.length > 0" ref="activitiesRef" class="member-activities" @scroll.passive="onScroll">
```

- [ ] **Step 2: 运行组件测试**

Run: `pnpm --dir web test -- run src/components/chat/v2/__tests__/MemberSessionPanel.spec.ts`
Expected: PASS（4 个用例全过，无 composable mock 需要更新）。

- [ ] **Step 3: 删除旧 composable**

先全局确认无其他引用：

Run: `pnpm --dir web exec rg -l "useActivityAutoScroll" src`
Expected: 无输出（仅 MemberSessionPanel 曾引用，已替换）。

然后删除文件 `web/src/features/chat/composables/useActivityAutoScroll.ts`。

- [ ] **Step 4: 验证**

Run: `pnpm --dir web test -- run src/features/chat src/components/chat`
Expected: PASS（无 import 残留错误）。

---

## Task 4: useActivityQueries 增加 teamRuns() + ChatMessagePanel 接线（修复 P1）

**Files:**
- Modify: `web/src/features/chat/composables/useActivityQueries.ts:90-103`
- Modify: `web/src/components/chat/ChatMessagePanel.vue:135,141,284,492-496`

- [ ] **Step 1: useActivityQueries 增加 teamRuns 访问器**

在 [useActivityQueries.ts](file:///f:/aranea-agents/web/src/features/chat/composables/useActivityQueries.ts#L95-L98) 的 `teamStages()` 之后追加（`TeamRun` 类型已在文件头部 import）：

```ts
    /** Read-only view of the teamRuns map. */
    teamRuns(): ReadonlyMap<string, TeamRun> {
      return store.teamRuns;
    },
```

- [ ] **Step 2: ChatMessagePanel 组装签名并接入**

[ChatMessagePanel.vue](file:///f:/aranea-agents/web/src/components/chat/ChatMessagePanel.vue#L283-L284) import 区（第 283-284 行附近），在 `useTodoBoard` import 之后追加：

```ts
import { useActivityQueries } from '../../features/chat/composables/useActivityQueries';
```

将第 492-496 行：

```ts
const { showScrollBtn, onMessagesScroll, scrollToBottom, scrollToTurnId } = useChatMessageScroll({
  sessionKey,
  messages: messagesRef,
  messagesScrollEl,
});
```

替换为：

```ts
// ── 状态制跟随：组装活动树末端签名（O(1)，不遍历全树）──
// 外层视口跟随语义 = "跟随主流尾部"：最后一个 task 的最后一个 turn 的最后一步。
// 历史 task/turn 的局部更新（如重新生成）不改变签名，不触发外层跟随。
const activityQueries = useActivityQueries();
const sessionTasks = computed(() => (props.sessionId ? activityQueries.getSessionTasks(props.sessionId) : []));
const lastTaskId = computed(() => sessionTasks.value[sessionTasks.value.length - 1]?.ID ?? '');
const chatContentSignature = computed(() => {
  const tasks = sessionTasks.value;
  const lastTask = tasks[tasks.length - 1];
  const turns = lastTask ? activityQueries.getTaskTurns(lastTask.ID) : [];
  const lastTurn = turns[turns.length - 1];
  const steps = lastTurn ? activityQueries.getTurnSteps(lastTurn.ID) : [];
  const lastStep = steps[steps.length - 1];
  const msgs = props.messages;
  const lastMsg = msgs[msgs.length - 1];
  return [
    msgs.length,
    lastMsg?.content_markdown?.length ?? 0,
    tasks.length,
    steps.length,
    lastStep?.ID ?? '',
    lastStep?.Status ?? '',
    lastStep?.Content?.length ?? 0,
    activityQueries.teamStages().size,
    activityQueries.teamRuns().size,
  ].join(':');
});

const { onMessagesScroll, scrollToBottom, scrollToTurnId } = useChatMessageScroll({
  sessionKey,
  messages: messagesRef,
  messagesScrollEl,
  contentSignature: chatContentSignature,
  lastTaskId,
});
```

**注意**：`computed` 已在文件头部从 vue import（第 265 行），无需新增。`scrollToBottom` 仍被模板 `@scroll-to-bottom` 引用，Task 5 移除该绑定前保留解构。

- [ ] **Step 3: 运行类型检查**

Run: `pnpm --dir web exec vue-tsc --noEmit -p tsconfig.json 2>&1 | Select-String -Pattern 'ChatMessagePanel'`
Expected: 仅剩模板侧 `showScrollBtn` 未定义 / `show-scroll-btn` prop 相关错误（Task 5 修复）；script 部分无类型错误。

---

## Task 5: ChatMessageList 移除 ↓ 回底按钮 + ChatMessagePanel 模板清理

**Files:**
- Modify: `web/src/components/chat/ChatMessageList.vue:54-65,93,105`
- Modify: `web/src/components/chat/ChatMessagePanel.vue:135,141,492`（模板两行 + script 解构）

- [ ] **Step 1: ChatMessageList 删除按钮块**

删除 [ChatMessageList.vue](file:///f:/aranea-agents/web/src/components/chat/ChatMessageList.vue#L54-L65) 第 54-65 行：

```vue
    <transition name="chat-scroll-fade">
      <q-btn
        v-if="showScrollBtn"
        round
        unelevated
        color="accent"
        icon="arrow_downward"
        class="chat-scroll-bottom"
        :aria-label="t('chat.scrollToLatest')"
        @click="$emit('scroll-to-bottom', true)"
      />
    </transition>
```

- [ ] **Step 2: ChatMessageList 删除 prop 与 emit**

删除 props 中第 93 行：

```ts
  showScrollBtn: boolean;
```

删除 emits 中第 105 行：

```ts
  'scroll-to-bottom': [smooth: boolean];
```

- [ ] **Step 3: ChatMessagePanel 模板清理**

删除 [ChatMessagePanel.vue](file:///f:/aranea-agents/web/src/components/chat/ChatMessagePanel.vue#L135) 第 135 行：

```vue
            :show-scroll-btn="showScrollBtn"
```

删除第 141 行：

```vue
            @scroll-to-bottom="scrollToBottom"
```

同时将 Task 4 Step 2 的解构中的 `scrollToBottom` 移除（已无引用）：

```ts
const { onMessagesScroll, scrollToTurnId } = useChatMessageScroll({
```

- [ ] **Step 4: 类型检查 + 全量前端单测**

Run: `pnpm --dir web exec vue-tsc --noEmit -p tsconfig.json`
Expected: exit 0，无 `showScrollBtn` / `scroll-to-bottom` 相关错误。

Run: `pnpm --dir web test -- run`
Expected: 全量 PASS。

---

## Task 6: 左边线状态样式（TeamStagePanel / TeamRunCard / MemberSessionPanel）

**Files:**
- Modify: `web/src/components/chat/v2/TeamStagePanel.vue:68-74`
- Modify: `web/src/components/chat/v2/TeamRunCard.vue:11,295-302,514-516`
- Modify: `web/src/components/chat/v2/MemberSessionPanel.vue:10,287-292`

**状态色映射（规格 §4.2，CSS 变量昼夜双主题已存在于 `_css-vars-light/dark.sass`）：**

| 状态 | 左边线 | 动画 |
|------|--------|------|
| running | `var(--color-accent)` | 1.6s 呼吸脉冲 |
| paused | `var(--color-warning)` | — |
| completed | `var(--color-success)` | — |
| failed | `var(--color-danger)` | — |
| cancelled / skipped | `var(--color-text-tertiary)` | — |
| pending（仅成员） | `var(--glass-border)` | — |

- [ ] **Step 1: TeamStagePanel 去盒子**

[TeamStagePanel.vue](file:///f:/aranea-agents/web/src/components/chat/v2/TeamStagePanel.vue#L68-L74) style 块整体替换为：

```sass
<style lang="sass" scoped>
// 2026-07-22 左边线体系：纯语义容器，无边框/背景（保留 data-team-stage-id 与定位高亮）
.team-stage-panel
  margin: 8px 0
</style>
```

- [ ] **Step 2: TeamRunCard 左边线 + 状态类名绑定**

模板第 11 行：

```vue
  <div class="team-run-card" :data-team-run-id="teamRun.ID">
```

改为：

```vue
  <div class="team-run-card" :class="`team-run-card--${teamRun.Status}`" :data-team-run-id="teamRun.ID">
```

style 块中 `.team-run-card` 定义（第 297-302 行）：

```sass
.team-run-card
  border: 1px solid var(--glass-border)
  border-radius: 6px
  margin: 4px 0
  background: var(--glass-surface)
  overflow: hidden
```

替换为：

```sass
// 2026-07-22 左边线体系：去四周边框+背景，3px 左状态线 + 左内边距
.team-run-card
  margin: 4px 0
  padding-left: 10px
  border-left: 3px solid var(--color-text-tertiary)

  &--running
    border-left-color: var(--color-accent)
    animation: team-run-border-pulse 1.6s ease-in-out infinite

  &--paused
    border-left-color: var(--color-warning)

  &--completed
    border-left-color: var(--color-success)

  &--failed
    border-left-color: var(--color-danger)

  &--cancelled
    border-left-color: var(--color-text-tertiary)

@keyframes team-run-border-pulse
  0%, 100%
    border-left-color: var(--color-accent)
  50%
    border-left-color: color-mix(in srgb, var(--color-accent) 35%, transparent)
```

`.team-run-expand`（第 514-516 行）去掉顶部边框线（去盒子化，成员区挂在团队左边线下自然分层）：

```sass
.team-run-expand
  padding: 6px 10px
  border-top: 1px solid var(--glass-border)
```

替换为：

```sass
.team-run-expand
  padding: 6px 10px
```

**不改**：`.team-run-bar:hover` 的 `background: var(--glass-surface-hover)`（规格要求 hover 出微弱玻璃背景，已存在）；`.team-run-bar__header/__middle` 的 `border-right` 内部分隔线保留。

- [ ] **Step 3: MemberSessionPanel 左边线 + 缩进**

模板第 10 行：

```vue
  <div class="member-session-panel" :data-agent-key="memberSession.AgentKey">
```

改为：

```vue
  <div class="member-session-panel" :class="`member-session-panel--${memberSession.Status}`" :data-agent-key="memberSession.AgentKey">
```

style 块中 `.member-session-panel`（第 288-292 行）：

```sass
.member-session-panel
  border: 1px solid var(--glass-border)
  border-radius: 4px
  margin: 4px 0
  background: var(--glass-surface)
```

替换为：

```sass
// 2026-07-22 左边线体系：去边框+背景，3px 左状态线 + margin-left 14px（挂在团队线之下形成视觉树）
.member-session-panel
  margin: 4px 0 4px 14px
  padding-left: 10px
  border-left: 3px solid var(--glass-border)

  &--running
    border-left-color: var(--color-accent)
    animation: member-border-pulse 1.6s ease-in-out infinite

  &--paused
    border-left-color: var(--color-warning)

  &--completed
    border-left-color: var(--color-success)

  &--failed
    border-left-color: var(--color-danger)

  &--cancelled, &--skipped
    border-left-color: var(--color-text-tertiary)

@keyframes member-border-pulse
  0%, 100%
    border-left-color: var(--color-accent)
  50%
    border-left-color: color-mix(in srgb, var(--color-accent) 35%, transparent)
```

**不改**：`.member-header:hover` 的玻璃 hover；`.member-activities` 的 300px max-height 与细滚动条（既有约束）。

- [ ] **Step 4: 组件测试 + 构建验证**

Run: `pnpm --dir web test -- run src/components/chat`
Expected: PASS。

Run: `pnpm --dir web build`
Expected: 构建成功（sass `color-mix` 与 `@keyframes` 编译通过）。

---

## Task 7: 清理 i18n 与全局样式残留

**Files:**
- Modify: `web/src/i18n/locales/zh-CN.ts:518`
- Modify: `web/src/i18n/locales/en-US.ts:502`
- Modify: `web/src/css/theme/_chat-message-panel.sass:733-760,1202-1206`

> 已 Grep 确认 `chat.scrollToLatest` 仅被已删除的 ChatMessageList 按钮引用；`.chat-scroll-bottom` / `.chat-scroll-fade` 仅存在于本 sass 文件。

- [ ] **Step 1: 删除 i18n key**

[zh-CN.ts](file:///f:/aranea-agents/web/src/i18n/locales/zh-CN.ts#L518) 删除：

```ts
    scrollToLatest: '滚动到最新消息',
```

[en-US.ts](file:///f:/aranea-agents/web/src/i18n/locales/en-US.ts#L502) 删除：

```ts
    scrollToLatest: 'Scroll to latest message',
```

- [ ] **Step 2: 删除 sass 样式块**

[_chat-message-panel.sass](file:///f:/aranea-agents/web/src/css/theme/_chat-message-panel.sass#L733-L760) 删除第 733 行注释到 `.chat-scroll-fade-leave-to` 规则结束（约第 760 行，含 `// ===== Scroll-to-bottom button =====` 注释、`.chat-scroll-bottom` 全部规则、`.chat-scroll-fade-*` transition 规则）；以及响应式段中第 1202-1206 行：

```sass
  .chat-scroll-bottom
    right: 12px
    bottom: 12px
    width: 40px
    height: 40px
```

- [ ] **Step 3: 全局确认无残留**

Run: `pnpm --dir web exec rg -n "scrollToLatest|chat-scroll-bottom|chat-scroll-fade|showScrollBtn|useActivityAutoScroll" src`
Expected: 无输出。

- [ ] **Step 4: lint + test + build**

Run: `pnpm --dir web lint`
Expected: PASS。

Run: `pnpm --dir web test -- run`
Expected: 全量 PASS。

Run: `pnpm --dir web build`
Expected: 构建成功。

---

## Task 8: 文档同步（DOC-SYNC 红线）

**Files:**
- Modify: `docs/development/1-chat.design.md`（B.2.2 滚动锚点、B.4.5 自动滚动模型、新增 B.4.6）

- [ ] **Step 1: 更新 B.2.2 滚动锚点（第 3246-3250 行）**

将：

```markdown
**滚动锚点**：

- 新 Turn 开始时，自动滚动到 UserMessageBubble 顶部（用户能立即看到自己的消息）
- 进行中的 Turn（如 streaming）保持滚动跟随最新内容
- 已完成的 Turn 不自动滚动（用户可手动向上浏览历史）
```

替换为：

```markdown
**滚动锚点**（2026-07-22 状态制重构后落地）：

- 新 Task 创建时（用户发送消息），自动滚动到 TaskCard 的 UserMessage 顶部（`[data-task-id]`，`scrollIntoView({block:'start'})`），用户能立即看到自己的消息
- FOLLOWING 状态（用户位于距底 ≤80px）时，streaming / 团队执行新内容实时滚底跟随（50ms leading+trailing rAF 节流，内容签名为活动树末端 O(1) 签名）
- 用户滚离底部（UNFOLLOWED）后**永不**自动滚动，无定时器强拽；滚回底部即恢复跟随
- 跟随中容器内出现非空选区（用户复制文字）→ 暂停跟随，滚回底部恢复
- 实现：`web/src/features/chat/composables/useFollowScroll.ts` + `useChatMessageScroll.ts`
```

- [ ] **Step 2: 更新 B.4.5 agent-card 自动滚动模型（第 3751-3757 行）**

将：

```markdown
**agent-card 自动滚动模型**（2026-07-05 新增）：
- 触发条件：`!collapsed && status === 'running'`（展开 + 运行中）
- 实时滚动：内容变化（新 step 或最后一步内容增长）时自动滚到底部
- 用户意图优先：用户滚动离开底部时进入 10s 冷却期，期间不自动滚动
- 恢复机制：10s 内无用户滚动 → 自动滚到底部并恢复实时跟踪；用户滚回底部时立即恢复
- 实现位置：`web/src/features/chat/composables/useActivityAutoScroll.ts`
- 与 `useChatMessageScroll` 的区别：后者使用阈值模型（80px 内 stick），前者使用时间模型（10s 恢复），符合"用户不操作了 10s 再自动刷新"的需求
```

替换为：

```markdown
**agent-card 自动滚动模型**（2026-07-22 状态制重构，取代 2026-07-05 时间制模型）：
- 触发条件：`!collapsed && status === 'running'`（展开 + 运行中）
- 状态制跟随：与外层主聊天共用 `useFollowScroll`——FOLLOWING 中内容变化（新 step 或最后一步内容增长）实时滚底；用户滚离底部（>80px）转 UNFOLLOWED，**永不**自动滚动，滚回底部（≤80px）即恢复
- 选区保护：跟随中容器内出现非空选区 → 暂停跟随（保护复制）
- 展开 / agent 启动（enabled false→true）→ 滚底并进入 FOLLOWING；折叠 / 终态 → 停止跟随，不动滚动条
- 实现位置：`web/src/features/chat/composables/useFollowScroll.ts`（`useActivityAutoScroll.ts` 已删除）
- 与外层 `useChatMessageScroll` 的关系：后者是 `useFollowScroll` 的薄封装（末端签名 + 新 Task 锚点 + turn 定位高亮），两层共用同一状态机
```

- [ ] **Step 3: B.4.5 之后新增 B.4.6 左边线状态样式体系**

在 B.4.5 末尾（第 3768 行 `member-action-pulse` 段之后、`### B.5` 之前）插入：

```markdown
#### B.4.6 团队区域左边线状态样式体系（2026-07-22 新增）

> 取代三层边框盒套娃（TeamStagePanel[盒] → TeamRunCard[盒] → MemberSessionPanel[盒]）。学习 Cursor/Trae 的「左侧竖线 + 状态色 + 缩进」模式，**DOM 结构不变，仅改样式**。

**层级规则**：

| 元素 | 样式 |
|------|------|
| 精灵主流 steps（TurnContainer 直接子级） | 无左边线——无线 = 主流 |
| TeamStagePanel | 纯语义容器，无边框/背景（保留 `data-team-stage-id` 与 `activity-locate-highlight`） |
| TeamRunCard | 3px 左状态线 + `padding-left: 10px`；hover 出 `--glass-surface-hover` 微弱背景 |
| MemberSessionPanel | 3px 左状态线 + `margin-left: 14px`（挂在团队线之下形成视觉树） |

**状态色映射**（左边线与状态徽章同色呼应）：

| 状态 | 左边线 | 动画 |
|------|--------|------|
| running | `var(--color-accent)` | 1.6s 呼吸脉冲（承担原"新动态呼吸点"语义） |
| paused | `var(--color-warning)` | — |
| completed | `var(--color-success)` | — |
| failed | `var(--color-danger)` | — |
| cancelled / skipped | `var(--color-text-tertiary)` | — |
| pending（仅成员） | `var(--glass-border)` | — |

**背景策略**：容器背景全部去除；成员 ReplyBlock 气泡（`--glass-elevated`）在无色容器上自然浮出，"成员说了什么"不靠提级结构即可辨识。

**内外层级辨识**：有左边线 = 团队子执行树（内部）；无线 = 精灵主流（外部）。
```

- [ ] **Step 4: 文档自检（aranea-docs-guide §8）**

确认：改动均为既有模块文档的内容更新（无新文件、无命名问题、无子文档拆分）；B.2.2/B.4.5/B.4.6 属设计文档允许的「前端组件设计/UX 规范」内容边界。

---

## Task 9: 运行时验证（R3，必须）

- [ ] **Step 1: 启动前后端**

后端：`go build ./... && go run ./cmd/admin`（或既有运行实例）；前端：`pnpm --dir web dev`。

- [ ] **Step 2: 逐条目检（对应规格 §7 运行时验证清单）**

| # | 操作 | 预期 |
|---|------|------|
| 1 | 发送消息触发精灵流式 | 跟随中实时滚底；新 Task 出现时先锚定到 UserMessage 顶部 |
| 2 | 触发团队执行（多成员） | 外层跟随 TeamRunCard 增长；成员面板 300px 区跟随 |
| 3 | 流式期间滚离底部阅读历史 | 永不打扰；等 10s+ 也不拽回；滚回底部恢复跟随 |
| 4 | 跟随中选中文字复制 | 不滚动；滚回底部恢复 |
| 5 | 目检左边线 | running 蓝线脉冲 / completed 绿 / failed 红 / cancelled 灰；精灵主流无线；成员面板相对团队卡缩进 14px |
| 6 | 折叠成员面板再展开 | 展开滚底跟随；折叠期间不滚动 |
| 7 | 切换会话 | 滚底 + 恢复跟随 |

- [ ] **Step 3: 日志检查**

读 `logs/aranea-pipeline.log` 确认无异常错误。

---

## Self-Review 记录

**规格覆盖核对：**
- §3.1 useFollowScroll → Task 1 ✅
- §3.2 外层重写（末端签名/新 Task 锚点/删 showScrollBtn/保留 turn 定位） → Task 2 + Task 4 ✅
- §3.3 内层 MemberSessionPanel 换用 → Task 3 ✅
- §3.2「在 ChatMessagePanel 组装后传入——修复 P1」 → Task 4 ✅
- §4 左边线样式（TeamStagePanel/TeamRunCard/MemberSessionPanel） → Task 6 ✅
- §5 改动清单：删 useActivityAutoScroll → Task 3 Step 3 ✅；ChatMessageList 按钮 → Task 5 ✅；i18n 清理 → Task 7 ✅（sass 残留一并清理，Grep 已验证）
- §6 边界处理（null no-op / clamp / 选区容器判定 / 卸载清理 / enabled 语义） → Task 1 实现 + 测试 ✅
- §7 测试计划（单测 + 运行时验证） → Task 1/2 单测 + Task 9 ✅
- DOC-SYNC（1-chat.design.md） → Task 8 ✅

**类型一致性核对：**
- `useFollowScroll` 返回 `{ following, onScroll, jumpToLatest, scrollToBottom }` —— Task 2 使用 `follow.jumpToLatest / follow.scrollToBottom / follow.onScroll` ✅
- `useChatMessageScroll` 返回 `{ highlightedTurnId, onMessagesScroll, scrollToBottom, scrollToTurnId }` —— Task 4 解构 `onMessagesScroll, scrollToBottom(临时), scrollToTurnId`，Task 5 移除 `scrollToBottom` ✅
- `teamRuns()` 访问器（Task 4 Step 1）与 ChatMessagePanel 签名组装（Task 4 Step 2）一致 ✅
- 状态类名 `team-run-card--${Status}` / `member-session-panel--${Status}` 与 sass `&--running` 等修饰类一致（成员含 `pending`，团队无 `pending`/`skipped`——TeamRun.Status 枚举无这两个值）✅
