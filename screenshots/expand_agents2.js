(() => {
  // Click all agent-card headers to expand them
  const agentHeaders = Array.from(document.querySelectorAll('.agent-card__header'));
  let clicked = 0;
  for (const h of agentHeaders) {
    try {
      h.click();
      clicked++;
    } catch (e) {}
  }
  return JSON.stringify({ clicked, headerCount: agentHeaders.length });
})()
