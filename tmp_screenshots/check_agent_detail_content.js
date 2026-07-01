(function() {
  const team = document.querySelector('.team-card--expanded');
  if (!team) return 'no expanded team';
  const agent = team.querySelector('.agent-card');
  if (!agent) return 'no agent-card';
  const detail = agent.querySelector('.agent-card__detail');
  if (!detail) return 'no detail';
  const plans = detail.querySelectorAll('.plan-block');
  const graphStages = detail.querySelectorAll('.graph-stage-block');
  const teamCards = detail.querySelectorAll('.team-card');
  const blocks = Array.from(detail.children).map(c => ({
    tag: c.tagName,
    class: c.className,
    text: c.textContent.slice(0, 80).replace(/\s+/g, ' ')
  }));
  return JSON.stringify({ plans: plans.length, graphStages: graphStages.length, teamCards: teamCards.length, blocks }, null, 2);
})();
