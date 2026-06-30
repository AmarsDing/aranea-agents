(() => {
  // Check team-card bar-fill widths
  const fills = document.querySelectorAll('.team-card__bar-fill');
  const fillInfo = [];
  fills.forEach((f, i) => {
    fillInfo.push({
      index: i,
      width: f.style.width || window.getComputedStyle(f).width,
      parent: f.parentElement?.className || '',
    });
  });

  // Check team-card status text
  const statuses = document.querySelectorAll('.team-card__status');
  const statusInfo = [];
  statuses.forEach((s, i) => {
    statusInfo.push({
      index: i,
      text: s.textContent?.trim() || '',
      class: s.className || '',
    });
  });

  // Find AgentCard components - check actual class names
  const allCards = document.querySelectorAll('[class*="card"]');
  const cardClasses = new Set();
  allCards.forEach(c => {
    const cls = c.className || '';
    if (typeof cls === 'string' && cls.includes('card')) {
      cls.split(/\s+/).forEach(c => { if (c.includes('card') && c.length < 50) cardClasses.add(c); });
    }
  });

  // Check session-stage / agent-card elements
  const sessionStages = document.querySelectorAll('.session-stage, [class*="session-stage"]');
  const sessionInfo = [];
  sessionStages.forEach((s, i) => {
    sessionInfo.push({
      index: i,
      class: s.className?.substring(0, 100) || '',
      text: s.textContent?.trim()?.substring(0, 80) || '',
    });
  });

  return JSON.stringify({
    barFills: fillInfo,
    teamStatuses: statusInfo,
    cardClasses: Array.from(cardClasses).slice(0, 20),
    sessionStages: sessionInfo.slice(0, 8),
  }, null, 2);
})()
