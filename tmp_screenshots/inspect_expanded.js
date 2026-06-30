(() => {
  // After expanding, find all child elements with agent-card or session-stage
  const firstTeamCard = document.querySelector('.team-card');
  if (!firstTeamCard) return 'No team-card found';
  
  // Check expanded state
  const isExpanded = firstTeamCard.classList.contains('team-card--expanded');
  
  // Find children with various possible agent-card classes
  const allChildren = firstTeamCard.querySelectorAll('*');
  const agentClasses = new Set();
  const sessionElements = [];
  
  for (const child of allChildren) {
    const cls = typeof child.className === 'string' ? child.className : '';
    if (cls.includes('agent') || cls.includes('session') || cls.includes('member')) {
      cls.split(/\s+/).forEach(c => {
        if (c.length > 0 && c.length < 60) agentClasses.add(c);
      });
    }
    if (cls.includes('session-stage') || cls.includes('agent-card')) {
      sessionElements.push({
        class: cls.substring(0, 100),
        text: child.textContent?.trim()?.substring(0, 100) || '',
        status: child.querySelector('[class*="status"]')?.textContent?.trim() || '',
      });
    }
  }
  
  // Find all .team-card__member elements and their status
  const members = firstTeamCard.querySelectorAll('.team-card__member');
  const memberInfo = [];
  members.forEach((m, i) => {
    const cls = m.className || '';
    const statusMatch = cls.match(/--(\w+)$/);
    memberInfo.push({
      index: i,
      name: m.querySelector('.team-card__member-name')?.textContent?.trim() || '',
      statusClass: statusMatch ? statusMatch[1] : '',
      hasCheck: !!m.querySelector('.team-card__member-mark'),
      hasCross: !!m.querySelector('.team-card__member-mark--fail'),
      hasDot: !!m.querySelector('.team-card__member-dot--running'),
    });
  });
  
  return JSON.stringify({
    isExpanded,
    agentClasses: Array.from(agentClasses).slice(0, 25),
    sessionElements: sessionElements.slice(0, 10),
    members: memberInfo,
  }, null, 2);
})()
