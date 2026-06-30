(() => {
  const headers = Array.from(document.querySelectorAll('.agent-card > .agent-card__row > .agent-card__header'));
  for (const h of headers) {
    h.click();
  }
  return JSON.stringify({ clicked: headers.length });
})()
