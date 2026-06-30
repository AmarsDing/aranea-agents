(function() {
  const card = document.querySelector('.agent-card--expanded') || document.querySelector('.agent-card');
  if (!card) return 'no agent-card';
  const detail = card.querySelector('.agent-card__detail, [class*="detail"]') || card;
  detail.scrollIntoView({ behavior: 'instant', block: 'start' });
  return 'scrolled agent detail';
})();
