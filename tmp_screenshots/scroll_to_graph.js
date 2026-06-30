// Scroll to the graph-stage or team-card area
(function() {
  const el = document.querySelector('.graph-stage-block') || document.querySelector('.team-card') || document.querySelector('.agent-card');
  if (el) {
    el.scrollIntoView({ behavior: 'instant', block: 'start' });
    return 'scrolled to ' + (el.className || el.tagName);
  }
  return 'no graph-stage/team-card/agent-card found';
})();
