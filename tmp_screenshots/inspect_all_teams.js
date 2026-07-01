(function() {
  const cards = document.querySelectorAll('.team-card');
  return JSON.stringify(Array.from(cards).map((c, i) => ({
    index: i,
    className: c.className,
    height: c.offsetHeight,
    top: c.getBoundingClientRect().top,
    detailHeight: c.querySelector('.team-card__detail')?.offsetHeight || 0,
    agentCards: c.querySelectorAll('.agent-card').length
  })), null, 2);
})();
