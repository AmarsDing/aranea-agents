(function() {
  const stream = document.querySelector('.event-stream');
  if (!stream) return 'no stream';
  const children = Array.from(stream.children).map(c => ({
    tag: c.tagName,
    class: c.className,
    text: c.textContent.slice(0, 80).replace(/\s+/g, ' ')
  }));
  // Also check recursively for team-card
  const teamCards = Array.from(stream.querySelectorAll('.team-card, [class*="team-card"]')).map(c => c.className);
  const graphStages = Array.from(stream.querySelectorAll('.graph-stage-block')).map(c => c.className);
  return JSON.stringify({ childCount: stream.children.length, children, teamCards, graphStages }, null, 2);
})();
