(function() {
  const card = document.querySelector('.team-card');
  if (!card) return 'no team-card';
  const detail = card.querySelector('.team-card__detail');
  const row = card.querySelector('.team-card__row');
  return JSON.stringify({
    className: card.className,
    teamCardHeight: card.offsetHeight,
    teamCardRect: card.getBoundingClientRect(),
    rowRect: row ? row.getBoundingClientRect() : null,
    detailRect: detail ? detail.getBoundingClientRect() : null,
    detailVisible: detail ? detail.offsetParent !== null : false,
    agentCardsInDetail: detail ? detail.querySelectorAll('.agent-card').length : 0
  }, null, 2);
})();
