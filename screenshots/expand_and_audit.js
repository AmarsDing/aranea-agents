// Expand all TeamCards and then audit AgentCard statuses
(() => {
  // Find all TeamCard headers and click to expand
  const teamHeaders = document.querySelectorAll('.team-card__header');
  let expanded = 0;
  for (const h of teamHeaders) {
    // Check if already expanded
    const card = h.closest('.team-card');
    if (card && !card.classList.contains('team-card--expanded')) {
      // Simulate click with proper event
      const evt = new MouseEvent('click', { bubbles: true, cancelable: true, view: window });
      h.dispatchEvent(evt);
      expanded++;
    }
  }

  // Wait a bit for Vue to react, then return status
  return JSON.stringify({ expandedCount: expanded, totalHeaders: teamHeaders.length });
})();
