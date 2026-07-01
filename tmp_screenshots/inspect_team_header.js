(function() {
  const card = document.querySelector('.team-card');
  if (!card) return 'no team-card';
  const header = card.querySelector('.team-card__header');
  if (!header) return 'no header';
  const cs = getComputedStyle(header);
  return JSON.stringify({
    headerExists: true,
    headerTag: header.tagName,
    headerClass: header.className,
    cursor: cs.cursor,
    pointerEvents: cs.pointerEvents,
    display: cs.display,
    clickListenerCount: header.onclick ? 1 : 0,
    vueListeners: header.__vueClick ? true : false,
    vueListeners2: header._vei ? Object.keys(header._vei || {}) : []
  }, null, 2);
})();
