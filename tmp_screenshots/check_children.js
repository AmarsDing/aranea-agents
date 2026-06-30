JSON.stringify({
  firstTeamCardChildren: (() => {
    const card = document.querySelector('.team-card');
    if (!card) return null;
    const stream = card.querySelector('.event-stream');
    if (!stream) return null;
    return Array.from(stream.children).map(child => ({
      tag: child.tagName,
      cls: child.className,
      text: child.textContent.trim().slice(0, 200),
      // Check for nested agent-card
      hasAgentCard: !!child.querySelector('.agent-card'),
      // Check for thinking/action/reply
      hasThinking: !!child.querySelector('[class*="thinking"], [class*="Thinking"]'),
      hasAction: !!child.querySelector('[class*="action-block"], [class*="ActionBlock"]'),
      hasReply: !!child.querySelector('[class*="reply"], [class*="Reply"]'),
      // Inner HTML preview
      html: child.outerHTML.slice(0, 800)
    }));
  })()
}, null, 2)
