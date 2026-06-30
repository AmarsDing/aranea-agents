(() => {
  // Find progress bars
  const progressBars = document.querySelectorAll('[class*="progress"]');
  const progressInfo = [];
  progressBars.forEach((p, i) => {
    const cls = p.className || '';
    const style = window.getComputedStyle(p);
    progressInfo.push({
      index: i,
      class: cls,
      width: style.width,
      text: p.textContent?.trim()?.substring(0, 50) || '',
    });
  });

  // Find agent-cards
  const agentCards = document.querySelectorAll('.agent-card, [class*="agent-card"]');
  const cardInfo = [];
  agentCards.forEach((c, i) => {
    const status = c.querySelector('[class*="status"]')?.textContent?.trim() || '';
    const name = c.querySelector('[class*="name"], [class*="title"]')?.textContent?.trim() || '';
    cardInfo.push({
      index: i,
      name,
      status,
      classes: c.className?.substring(0, 80) || '',
    });
  });

  // Find percentage text
  const pctTexts = [];
  document.querySelectorAll('*').forEach(el => {
    const t = el.textContent?.trim() || '';
    if (/^\d+%$/.test(t) && el.children.length === 0) {
      pctTexts.push({ text: t, class: el.className?.substring(0, 60) || '' });
    }
  });

  return JSON.stringify({
    progressBars: progressInfo.slice(0, 15),
    agentCards: cardInfo.slice(0, 12),
    pctTexts: pctTexts.slice(0, 15),
  }, null, 2);
})()
