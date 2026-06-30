// Audit TeamCard and AgentCard statuses
(() => {
  const result = { teams: [], agents: [] };

  // Check TeamCards
  const teamCards = document.querySelectorAll('.team-card');
  teamCards.forEach((tc, i) => {
    const name = tc.querySelector('.team-card__name, .team-card__title, [class*="team-card__name"]')?.textContent?.trim() || '?';
    const classes = tc.className;
    const status = classes.includes('team-card--completed') ? 'completed'
      : classes.includes('team-card--running') ? 'running'
      : classes.includes('team-card--failed') ? 'failed'
      : classes.includes('team-card--cancelled') ? 'cancelled'
      : classes.includes('team-card--paused') ? 'paused'
      : 'unknown';

    // Count agent cards inside this team
    const agentCards = tc.querySelectorAll('.agent-card');
    const agentStatuses = [];
    agentCards.forEach((ac) => {
      const acClasses = ac.className;
      const acStatus = acClasses.includes('agent-card--completed') ? 'completed'
        : acClasses.includes('agent-card--running') ? 'running'
        : acClasses.includes('agent-card--failed') ? 'failed'
        : acClasses.includes('agent-card--cancelled') ? 'cancelled'
        : acClasses.includes('agent-card--paused') ? 'paused'
        : 'unknown';
      const acName = ac.querySelector('.agent-card__name, [class*="agent-card__name"]')?.textContent?.trim() || '?';
      agentStatuses.push({ name: acName, status: acStatus });
    });

    result.teams.push({ index: i, name: name.substring(0, 40), status, agentCount: agentCards.length, agents: agentStatuses });
  });

  // Check standalone AgentCards (outside TeamCards)
  const allAgentCards = document.querySelectorAll('.agent-card');
  const standaloneAgents = [];
  allAgentCards.forEach((ac) => {
    if (!ac.closest('.team-card')) {
      const acClasses = ac.className;
      const acStatus = acClasses.includes('agent-card--completed') ? 'completed'
        : acClasses.includes('agent-card--running') ? 'running'
        : acClasses.includes('agent-card--failed') ? 'failed'
        : 'unknown';
      const acName = ac.querySelector('.agent-card__name, [class*="agent-card__name"]')?.textContent?.trim() || '?';
      standaloneAgents.push({ name: acName, status: acStatus });
    }
  });
  result.agents = standaloneAgents;

  return JSON.stringify(result, null, 2);
})();
