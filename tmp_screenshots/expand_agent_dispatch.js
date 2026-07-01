(function() {
  const expandedTeam = document.querySelector('.team-card--expanded');
  if (!expandedTeam) return 'no expanded team-card';
  const agentCard = expandedTeam.querySelector('.agent-card');
  if (!agentCard) return 'no agent-card in expanded team';
  const header = agentCard.querySelector('.agent-card__header') || agentCard;
  const evt = new MouseEvent('click', { bubbles: true, cancelable: true, view: window });
  header.dispatchEvent(evt);
  return 'dispatched click, classes=' + agentCard.className;
})();
