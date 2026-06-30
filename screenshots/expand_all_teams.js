(() => {
  // Find all team-card headers and click them programmatically
  const headers = Array.from(document.querySelectorAll('.team-card__header'));
  let clicked = 0;
  for (const h of headers) {
    // Check if parent team-card is already expanded
    const card = h.closest('.team-card');
    const isExpanded = card?.classList.contains('team-card--expanded');
    if (!isExpanded) {
      // Simulate a real click event
      const evt = new MouseEvent('click', { bubbles: true, cancelable: true, view: window });
      h.dispatchEvent(evt);
      clicked++;
    }
  }
  return JSON.stringify({ clicked, totalHeaders: headers.length });
})()
