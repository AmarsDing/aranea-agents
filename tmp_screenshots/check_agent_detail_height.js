(function() {
  const team = document.querySelector('.team-card--expanded');
  if (!team) return 'no expanded team';
  const agent = team.querySelector('.agent-card--expanded') || team.querySelector('.agent-card');
  if (!agent) return 'no agent-card';
  const detail = agent.querySelector('.agent-card__detail');
  const cs = detail ? getComputedStyle(detail) : null;
  return JSON.stringify({
    agentClass: agent.className,
    agentHeight: agent.offsetHeight,
    detailHeight: detail ? detail.offsetHeight : null,
    detailScrollHeight: detail ? detail.scrollHeight : null,
    detailMaxHeight: cs ? cs.maxHeight : null,
    detailOverflowY: cs ? cs.overflowY : null,
    detailDisplay: cs ? cs.display : null
  }, null, 2);
})();
