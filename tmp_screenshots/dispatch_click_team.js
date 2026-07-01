(function() {
  const card = document.querySelector('.team-card');
  if (!card) return 'no team-card';
  const header = card.querySelector('.team-card__header') || card;
  const evt = new MouseEvent('click', { bubbles: true, cancelable: true, view: window });
  header.dispatchEvent(evt);
  return 'dispatched click, classes=' + card.className;
})();
