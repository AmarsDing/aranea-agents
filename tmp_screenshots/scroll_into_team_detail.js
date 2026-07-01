(function() {
  const team = document.querySelector('.team-card--expanded');
  if (!team) return 'no expanded team';
  const detail = team.querySelector('.team-card__detail');
  if (detail) detail.scrollIntoView({ behavior: 'instant', block: 'start' });
  return 'scrolled into detail';
})();
