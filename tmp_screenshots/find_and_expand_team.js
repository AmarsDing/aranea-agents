(function() {
  const cards = document.querySelectorAll('.team-card');
  if (!cards.length) return 'no team-cards, stream exists=' + !!document.querySelector('.event-stream');
  // Expand the last team-card
  const team = cards[cards.length - 1];
  team.scrollIntoView({ behavior: 'instant', block: 'start' });
  const body = team.querySelector('.team-card__body');
  if (body) body.click();
  // Expand first agent-card inside
  setTimeout(() => {
    const agent = team.querySelector('.agent-card');
    if (agent) {
      const header = agent.querySelector('.agent-card__header');
      if (header) header.click();
    }
  }, 100);
  return 'found ' + cards.length + ' team-cards, clicked last body';
})();
