import { describe, expect, it } from 'vitest';
import { ref } from 'vue';
import { useChatScrollTitle } from '../useChatScrollTitle';
import type { Message } from '../types';
function setup(positions: number[]) {
  const root = document.createElement('div');
  root.getBoundingClientRect = () => ({ top: 100 }) as DOMRect;
  const markers = positions.map((top, index) => {
    const el = document.createElement('div');
    el.dataset.chatUserPrompt = `Task ${index + 1}`;
    el.getBoundingClientRect = () => ({ top, bottom: top + 20 }) as DOMRect;
    root.appendChild(el);
    return el;
  });
  const messages = ref<Message[]>([]);
  const api = useChatScrollTitle({
    sessionTitle: ref('Chat'),
    messages,
    messagesScrollEl: ref(root),
    virtualScrollRef: ref(null),
    useVirtualMessageList: ref(false),
  });
  return { api, markers, messages };
}
describe('reading context title', () => {
  it('keeps the preceding prompt while reading its reply, before the next prompt reaches the anchor', () => {
    const c = setup([0, 600]);
    c.api.refreshActivePrompt();
    expect(c.api.headerUserPrompt.value).toBe('Task 1');
  });
  it('changes only when the next prompt reaches the reading anchor', () => {
    const c = setup([-500, 160, 800]);
    c.api.refreshActivePrompt();
    expect(c.api.headerUserPrompt.value).toBe('Task 2');
  });
  it('uses the first upcoming prompt before any prompt reaches the anchor', () => {
    const c = setup([200, 600]);
    c.api.refreshActivePrompt();
    expect(c.api.headerUserPrompt.value).toBe('Task 1');
  });
  it('keeps the existing message fallback when no rendered markers exist', () => {
    const c = setup([]);
    c.messages.value = [{ role: 'user', content_markdown: 'Latest prompt' } as Message];
    c.api.refreshActivePrompt();
    expect(c.api.headerUserPrompt.value).toBe('Latest prompt');
  });
});
