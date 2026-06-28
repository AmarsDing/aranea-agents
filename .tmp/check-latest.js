(() => {
  // 滚动到底部
  const scrollContainer = document.querySelector('.chat-messages, .q-scroll, [class*="message-list"], [class*="event-stream"]') || document.querySelector('main');
  if (scrollContainer) {
    scrollContainer.scrollTop = scrollContainer.scrollHeight;
  }
  // 检查最新内容
  const allMessages = document.querySelectorAll('.event-stream > *, .chat-message, [class*="user-message"], [class*="reply-block"], [class*="thinking-block"]');
  const last10 = Array.from(allMessages).slice(-10).map((el) => ({
    tag: el.tagName,
    cls: (el.className || '').slice(0, 80),
    text: (el.textContent || '').trim().slice(0, 100),
  }));

  // 检查是否有停止按钮（说明还在生成）
  const stopBtn = document.querySelector('button[aria-label="停止生成"]');
  const sendBtn = document.querySelector('button[aria-label="发送"]');

  // 检查 textarea 值
  const textarea = document.querySelector('textarea');

  return JSON.stringify({
    scrollHeight: scrollContainer?.scrollHeight,
    scrollTop: scrollContainer?.scrollTop,
    hasStopButton: !!stopBtn,
    hasSendButton: !!sendBtn,
    sendDisabled: sendBtn?.disabled,
    textareaValue: textarea?.value?.slice(0, 80) || '',
    last10Elements: last10,
    totalEventStreamChildren: document.querySelectorAll('.event-stream > *').length,
  }, null, 2);
})();
