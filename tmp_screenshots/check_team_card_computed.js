(function() {
  const card = document.querySelector('.team-card');
  if (!card) return 'no team-card';
  const cs = getComputedStyle(card);
  return JSON.stringify({
    className: card.className,
    height: cs.height,
    display: cs.display,
    flexDirection: cs.flexDirection,
    overflow: cs.overflow,
    position: cs.position,
    dataAttrs: Array.from(card.attributes).map(a => `${a.name}=${a.value}`).slice(0, 10)
  }, null, 2);
})();
