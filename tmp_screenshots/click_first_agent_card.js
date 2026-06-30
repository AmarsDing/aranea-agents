(function() {
  const card = document.querySelector('.agent-card');
  if (!card) return 'no agent-card';
  const header = card.querySelector('.agent-card__header') || card;
  header.click();
  return 'clicked first agent-card header';
})();
