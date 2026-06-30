(function() {
  const card = document.querySelector('.team-card--expanded');
  if (!card) return 'no expanded team-card';
  const detail = card.querySelector('.team-card__detail');
  if (detail) {
    detail.scrollIntoView({ behavior: 'instant', block: 'start' });
    return 'scrolled to detail, visible=' + (detail.offsetParent !== null) + ' height=' + detail.offsetHeight;
  }
  return 'no detail found';
})();
