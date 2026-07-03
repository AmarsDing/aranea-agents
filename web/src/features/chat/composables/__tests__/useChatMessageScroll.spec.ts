// web/src/features/chat/composables/__tests__/useChatMessageScroll.spec.ts
import { describe, it, expect } from 'vitest';
import { ref } from 'vue';
import { useChatMessageScroll } from '../useChatMessageScroll';

describe('useChatMessageScroll v2', () => {
  it('accepts tasks ref instead of activityTree', () => {
    const { showScrollBtn, scrollToBottom } = useChatMessageScroll({
      sessionKey: ref('s1'),
      messages: ref([]),
      messagesScrollEl: ref(null),
      tasks: ref([]),
    });
    expect(showScrollBtn.value).toBe(false);
    expect(typeof scrollToBottom).toBe('function');
  });
});
