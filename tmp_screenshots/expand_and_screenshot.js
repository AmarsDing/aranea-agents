// Expand first team-card and scroll its top into view without shifting page too much
(function() {
  const card = document.querySelector('.team-card');
  if (!card) return 'no team-card';
  const header = card.querySelector('.team-card__header') || card;
  const evt = new MouseEvent('click', { bubbles: true, cancelable: true, view: window });
  header.dispatchEvent(evt);
  // Only scroll if card is way out of viewport
  const rect = card.getBoundingClientRect();
  if (rect.top < 0 || rect.top > window.innerHeight - 100) {
    card.scrollIntoView({ behavior: 'instant', block: 'start' });
  }
  return 'expanded, classes=' + card.className;
})();
