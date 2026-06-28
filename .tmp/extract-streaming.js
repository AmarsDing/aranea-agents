JSON.stringify({
  replyBlocks: Array.from(document.querySelectorAll('.reply-block')).map(function(e, i) {
    return {
      idx: i,
      classes: e.className,
      labelText: (e.querySelector('.reply-block__label-text') || {}).textContent || '',
      contentLen: (e.querySelector('.reply-block__content') || {}).textContent ? e.querySelector('.reply-block__content').textContent.length : 0,
      hasStreaming: /streaming|active|pending/i.test(e.className),
      lastContent: (e.querySelector('.reply-block__content') || {}).textContent ? e.querySelector('.reply-block__content').textContent.slice(-100) : ''
    };
  }).slice(-5),
  thinkingBlocks: Array.from(document.querySelectorAll('.thinking-block')).map(function(e, i) {
    return {
      idx: i,
      collapsed: /collapsed/.test(e.className),
      labelText: (e.querySelector('.thinking-block__label-text') || {}).textContent || '',
      preview: (e.querySelector('.thinking-block__preview') || {}).textContent ? e.querySelector('.thinking-block__preview').textContent.slice(0, 80) : ''
    };
  }).slice(-5),
  actActivities: Array.from(document.querySelectorAll('.act-activity')).map(function(e, i) {
    return {
      idx: i,
      status: (e.className.match(/act-activity__status--\w+/) || [])[0] || 'none',
      toolLabel: (e.querySelector('.act-activity__tool-label') || {}).textContent || '',
      duration: (e.querySelector('.act-activity__duration') || {}).textContent || ''
    };
  }).slice(-10),
  stopButton: !!document.querySelector('button:not([disabled])')
}, null, 2)
