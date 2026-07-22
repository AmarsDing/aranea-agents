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

  // G5 新 Task 锚点的会话归属跟踪：仅「同会话新增 Task」（用户发消息）时锚定到 TaskCard 顶部。
  // 会话切换 / 初始加载导致的 task 变化不锚定——由 alignMessageScroll 滚底负责，
  // 否则 scrollIntoView(block:'start') 会覆盖滚底（2026-07-22 运行时验证发现的会话切换 bug）。
  let lastSeenTaskId = opts.lastTaskId.value;

  // 会话切换：滚底 + FOLLOWING。
  // lastSeenTaskId 同步重置：会话切换后首次 lastTaskId 变化视为「初始加载」而非「新 Task」。
  watch(
    () => opts.sessionKey.value,
    () => {
      lastSeenTaskId = '';
      void alignMessageScroll();
    },
  );

  // G5 新 Task 锚点：同会话新增 Task → 锚定到 TaskCard（UserMessage）顶部 + FOLLOWING。
  watch(
    () => opts.lastTaskId.value,
    async (id, prev) => {
      if (!id || id === prev) return;
      if (!lastSeenTaskId) {
        lastSeenTaskId = id; // 初始加载：只记录，不锚定
        return;
      }
      lastSeenTaskId = id;
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
