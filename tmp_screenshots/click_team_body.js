(function() {
  const card = document.querySelector('.team-card');
  if (!card) return 'no team-card';
  const body = card.querySelector('.team-card__body');
  if (body) {
    body.click();
    return 'clicked body, classes=' + card.className;
  }
  return 'no body';
})();
