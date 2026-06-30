(() => {
  const teams = [];
  const agents = [];

  // Team cards
  const teamCardEls = document.querySelectorAll('.team-card');
  teamCardEls.forEach((card, i) => {
    const nameEl = card.querySelector('.team-card__title, .team-card__name, [class*="title"]');
    const statusEl = card.querySelector('.team-card__status, .team-card__status-badge, .q-chip, [class*="status-badge"]');
    const members = card.querySelectorAll('.agent-card').length;
    teams.push({
      index: i,
      name: nameEl ? nameEl.innerText.trim() : '(no name)',
      status: statusEl ? statusEl.innerText.trim() : '(no status)',
      memberCount: members
    });
  });

  // Agent cards
  const agentCardEls = document.querySelectorAll('.agent-card');
  agentCardEls.forEach((card, i) => {
    const nameEl = card.querySelector('.agent-card__name');
    const statusEl = card.querySelector('.agent-card__status-badge');
    const pulseEl = card.querySelector('.agent-card__pulse');
    const classList = Array.from(card.classList).filter(c => c.startsWith('agent-card--'));
    agents.push({
      index: i,
      name: nameEl ? nameEl.innerText.trim() : '(no name)',
      status: statusEl ? statusEl.innerText.trim() : '(no status)',
      statusClass: classList.join(',') || '(none)',
      isRunning: !!pulseEl || classList.includes('agent-card--running')
    });
  });

  return JSON.stringify({ teams: teams.length, agents: agents.length, teamDetails: teams, agentDetails: agents }, null, 2);
})()
