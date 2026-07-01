(function() {
  const card = document.querySelector('.team-card');
  if (!card) return 'no team-card';
  const header = card.querySelector('.team-card__header') || card;
  header.click();
  return 'clicked header, classes=' + card.className;
})();
