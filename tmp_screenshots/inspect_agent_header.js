(function() {
  const expandedTeam = document.querySelector('.team-card--expanded');
  if (!expandedTeam) return 'no expanded team-card';
  const agentCard = expandedTeam.querySelector('.agent-card');
  if (!agentCard) return 'no agent-card';
  const header = agentCard.querySelector('.agent-card__header');
  if (!header) return 'no header';
  return JSON.stringify({
    agentClass: agentCard.className,
    headerHTML: header.innerHTML.slice(0, 500),
    headerChildren: Array.from(header.children).map(c => ({ tag: c.tagName, class: c.className })),
    cs: {
      cursor: getComputedStyle(header).cursor,
      pointerEvents: getComputedStyle(header).pointerEvents
    }
  }, null, 2);
})();
