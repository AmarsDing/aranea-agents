(() => {
  const result = {
    replyBlocks: [],
    thinkingBlocks: [],
    actionBlocks: [],
    summary: {},
  };

  // 1. ReplyBlock: check streaming class
  const replies = document.querySelectorAll('.reply-block');
  replies.forEach((el, i) => {
    if (i < 5) {
      result.replyBlocks.push({
        className: el.className,
        hasStreamingClass: el.className.includes('reply-block--streaming'),
        labelText: el.querySelector('.reply-block__label-text')?.textContent?.trim() || '',
      });
    }
  });

  // 2. ThinkingBlock: check label-text content
  const thinkings = document.querySelectorAll('.thinking-block');
  thinkings.forEach((el, i) => {
    if (i < 8) {
      result.thinkingBlocks.push({
        labelText: el.querySelector('.thinking-block__label-text')?.textContent?.trim() || '',
        streamingText: el.querySelector('.thinking-block__streaming-text')?.textContent?.trim() || '',
        hasStreamingClass: el.className.includes('thinking-block--streaming'),
        className: el.className,
      });
    }
  });

  // 3. ActionBlock: check status class + tool label
  const actions = document.querySelectorAll('.act-activity');
  actions.forEach((el, i) => {
    if (i < 12) {
      const statusSpan = el.querySelector('.act-activity__status');
      const labelSpan = el.querySelector('.act-activity__tool-label');
      result.actionBlocks.push({
        labelText: labelSpan?.textContent?.trim() || '',
        statusText: statusSpan?.textContent?.trim() || '',
        statusClassName: statusSpan?.className || '',
        hasCancelledClass: statusSpan?.className?.includes('--cancelled') || false,
        hasFailedClass: statusSpan?.className?.includes('--failed') || false,
        hasSuccessClass: statusSpan?.className?.includes('--success') || false,
        durationText: el.querySelector('.act-activity__duration')?.textContent?.trim() || '',
      });
    }
  });

  result.summary = {
    totalReplies: replies.length,
    totalThinkings: thinkings.length,
    totalActions: actions.length,
    uniqueStatusClasses: [...new Set([...actions].map((a) => a.querySelector('.act-activity__status')?.className || '').filter((c) => c))],
    uniqueLabels: [...new Set([...actions].map((a) => a.querySelector('.act-activity__tool-label')?.textContent?.trim() || '').filter((l) => l))],
  };

  return JSON.stringify(result, null, 2);
})();
