(() => {
  // Click on the first agent-card header to expand it
  const agentCards = document.querySelectorAll('.agent-card');
  if (agentCards.length === 0) return JSON.stringify({ error: 'no agent cards' });
  
  // Click the first agent-card's header
  const header = agentCards[0].querySelector('.agent-card__header');
  if (header) {
    header.click();
    return JSON.stringify({ clicked: true, agentCardCount: agentCards.length });
  }
  return JSON.stringify({ clicked: false });
})()
