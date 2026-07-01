(function() {
  const graph = document.querySelector('.graph-stage-block');
  if (graph) graph.scrollIntoView({ behavior: 'instant', block: 'start' });
  const cards = document.querySelectorAll('.team-card');
  if (!cards.length) return 'no team-cards';
  // Expand the last team-card
  const team = cards[cards.length - 1];
  const teamBody = team.querySelector('.team-card__body');
  if (teamBody) teamBody.click();
  // Try to expand first agent-card inside it
  setTimeout(() => {
    const agent = team.querySelector('.agent-card');
    if (agent) {
      const header = agent.querySelector('.agent-card__header');
      if (header) header.click();
    }
  }, 100);
  return 'clicked team body';
})();
