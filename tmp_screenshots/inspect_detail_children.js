(function() {
  const team = document.querySelector('.team-card--expanded');
  if (!team) return 'no expanded team';
  const detail = team.querySelector('.team-card__detail');
  if (!detail) return 'no detail';
  const stream = detail.querySelector('.event-stream');
  if (!stream) return 'no event-stream';
  const children = Array.from(stream.children).map(c => ({
    tag: c.tagName,
    class: c.className,
    text: c.textContent.slice(0, 100).replace(/\s+/g, ' '),
    height: c.offsetHeight
  }));
  return JSON.stringify({ totalHeight: stream.offsetHeight, children }, null, 2);
})();
