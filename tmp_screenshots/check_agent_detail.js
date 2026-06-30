JSON.stringify({
  agentCards: Array.from(document.querySelectorAll('.agent-card')).map((a, i) => {
    const stream = a.querySelector('.event-stream');
    return {
      idx: i,
      status: a.className,
      name: a.querySelector('.agent-card__name')?.textContent.trim(),
      isExpanded: a.classList.contains('agent-card--expanded'),
      hasChildStream: !!stream,
      childCount: stream ? stream.children.length : 0,
      childClasses: stream ? Array.from(stream.children).map(c => c.className).slice(0, 5) : [],
      // Check for thinking/action/reply in child stream
      hasThinking: stream ? !!stream.querySelector('[class*="thinking"], [class*="Thinking"]') : false,
      hasAction: stream ? !!stream.querySelector('[class*="action-block"], [class*="ActionBlock"]') : false,
      hasReply: stream ? !!stream.querySelector('[class*="reply"], [class*="Reply"]') : false,
      // Footer buttons
      footerButtons: Array.from(a.querySelectorAll('.agent-card__footer button, .agent-card__actions button')).map(b => b.textContent.trim()),
      // Inject dialog
      hasInjectDialog: !!a.querySelector('.agent-card__inject, [class*="inject"]')
    };
  })
}, null, 2)
