(function() {
  const team = document.querySelector('.team-card--expanded');
  if (!team) return 'no expanded team';
  const agent = team.querySelector('.agent-card');
  if (!agent) return 'no agent-card';
  const header = agent.querySelector('.agent-card__header');
  if (header) {
    header.click();
    return 'toggled agent, agentClass=' + agent.className + ', teamClass=' + team.className;
  }
  return 'no header';
})();
