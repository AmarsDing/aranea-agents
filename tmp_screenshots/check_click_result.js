(function() {
  const cards = Array.from(document.querySelectorAll('.team-card')).map(c => c.className);
  return JSON.stringify(cards, null, 2);
})();
