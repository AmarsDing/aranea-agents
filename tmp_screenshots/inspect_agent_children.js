(() => {
  const agentCard = document.querySelector('.agent-card');
  if (!agentCard) return 'No agent-card found';
  
  const isExpanded = agentCard.classList.contains('agent-card--expanded');
  
  // Find all child blocks within the agent-card
  const children = agentCard.querySelectorAll('.agent-card__body, .agent-card__children, .thinking-block, .action-block, .reply-block, [class*="thinking"], [class*="action"], [class*="reply"]');
  const childInfo = [];
  children.forEach((c, i) => {
    childInfo.push({
      index: i,
      class: c.className?.substring(0, 80) || '',
      text: c.textContent?.trim()?.substring(0, 100) || '',
    });
  });
  
  // Check all direct children of agent-card
  const allChildren = agentCard.children;
  const directChildren = [];
  for (let i = 0; i < allChildren.length; i++) {
    directChildren.push({
      tag: allChildren[i].tagName,
      class: allChildren[i].className?.substring?.(0, 80) || '',
      childCount: allChildren[i].children.length,
    });
  }
  
  return JSON.stringify({
    isExpanded,
    directChildren,
    childBlocks: childInfo.slice(0, 15),
  }, null, 2);
})()
