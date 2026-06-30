(() => {
  // Expand all agent-cards
  const headers = Array.from(document.querySelectorAll('.agent-card__header'));
  let clicked = 0;
  for (const h of headers) {
    const card = h.closest('.agent-card');
    const isExpanded = card?.classList.contains('agent-card--expanded');
    if (!isExpanded) {
      const evt = new MouseEvent('click', { bubbles: true, cancelable: true, view: window });
      h.dispatchEvent(evt);
      clicked++;
    }
  }
  return JSON.stringify({ clicked, totalHeaders: headers.length });
})()
