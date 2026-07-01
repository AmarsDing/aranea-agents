(function() {
  const expandedTeam = document.querySelector('.team-card--expanded');
  if (!expandedTeam) return 'no expanded team-card';
  const agentCard = expandedTeam.querySelector('.agent-card');
  if (!agentCard) return 'no agent-card in expanded team';
  const header = agentCard.querySelector('.agent-card__header') || agentCard;
  header.click();
  return 'clicked agent header, classes=' + agentCard.className;
})();
