(() => {
  // Verify streaming reply class during active streaming
  const replies = document.querySelectorAll('.reply-block');
  const result = { replyBlocks: [], streamingActive: false };
  replies.forEach((el, i) => {
    if (i < 8) {
      const isStreaming = el.className.includes('reply-block--streaming');
      if (isStreaming) result.streamingActive = true;
      result.replyBlocks.push({
        className: el.className,
        isStreaming,
        labelText: el.querySelector('.reply-block__label-text')?.textContent?.trim() || '',
        contentLength: el.querySelector('.reply-block__markdown')?.textContent?.length || 0,
      });
    }
  });

  // Also check thinking blocks for streaming state
  const thinkings = document.querySelectorAll('.thinking-block');
  result.thinkingBlocks = [];
  thinkings.forEach((el, i) => {
    if (i < 5) {
      const isStreaming = el.className.includes('thinking-block--streaming');
      if (isStreaming) result.streamingActive = true;
      result.thinkingBlocks.push({
        isStreaming,
        labelText: el.querySelector('.thinking-block__label-text')?.textContent?.trim() || '',
        streamingText: el.querySelector('.thinking-block__streaming-text')?.textContent?.trim() || '',
      });
    }
  });

  result.totalReplies = replies.length;
  result.totalThinkings = thinkings.length;
  return JSON.stringify(result, null, 2);
})();
