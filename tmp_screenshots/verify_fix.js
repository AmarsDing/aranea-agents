(() => {
  const cards = document.querySelectorAll('.team-card');
  const result = [];
  cards.forEach((card, i) => {
    const status = card.querySelector('.team-card__status, [class*="status"]')?.textContent?.trim() || '';
    const title = card.querySelector('.team-card__title, .team-card__name, [class*="title"]')?.textContent?.trim() || '';
    const progressFill = card.querySelector('.team-card__progress-fill, .progress-fill, [class*="progress-fill"]');
    const progressText = card.querySelector('.team-card__progress-text, [class*="progress-text"]')?.textContent?.trim() || '';
    const members = card.querySelectorAll('.team-card__member, [class*="member"]');
    const memberInfo = [];
    members.forEach(m => {
      memberInfo.push({
        name: m.querySelector('[class*="name"], .agent-name')?.textContent?.trim() || '',
        status: m.className.match(/--(\w+)/)?.[1] || m.querySelector('[class*="status"]')?.textContent?.trim() || '',
      });
    });
    result.push({
      cardIndex: i,
      title,
      status,
      progressFillWidth: progressFill?.style?.width || progressFill?.getAttribute('width') || '',
      progressText,
      memberCount: members.length,
      members: memberInfo,
    });
  });
  return JSON.stringify(result, null, 2);
})()
