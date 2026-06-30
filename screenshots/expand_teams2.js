(() => {
  // Click all team-card headers to expand them
  const teamHeaders = Array.from(document.querySelectorAll('.team-card__header, .team-card__body'));
  let clicked = 0;
  for (const h of teamHeaders) {
    try {
      h.click();
      clicked++;
    } catch (e) {}
  }
  return JSON.stringify({ clicked, headerCount: teamHeaders.length });
})()
