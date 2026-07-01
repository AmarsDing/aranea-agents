(function() {
  const expanded = document.querySelector('.team-card--expanded');
  if (expanded) {
    expanded.scrollIntoView({ behavior: 'instant', block: 'start' });
    window.scrollBy(0, 300);
  }
  return 'scrolled';
})();
