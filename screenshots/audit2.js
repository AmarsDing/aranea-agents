(() => {
  const out = {};

  // Precise: exact class "team-card" or starts with "team-card " (with modifiers)
  function hasExactClass(el, name) {
    if (typeof el.className !== 'string') return false;
    return el.className.split(/\s+/).some(c => c === name || c.startsWith(name + '--'));
  }

  const teamCards = Array.from(document.querySelectorAll('div, section, article')).filter(el => hasExactClass(el, 'team-card'));
  const agentCards = Array.from(document.querySelectorAll('div, section, article')).filter(el => hasExactClass(el, 'agent-card'));
  const graphCards = Array.from(document.querySelectorAll('div, section, article')).filter(el => hasExactClass(el, 'graph-stage-block') || hasExactClass(el, 'graph-stage'));

  out.counts = { teamCards: teamCards.length, agentCards: agentCards.length, graphCards: graphCards.length };

  // Inspect each team-card
  out.teamCards = teamCards.map((el, i) => {
    const cls = el.className;
    const rect = el.getBoundingClientRect();
    // Header
    const nameEl = el.querySelector('.team-card__name');
    const taskEl = el.querySelector('.team-card__task');
    const timeEl = el.querySelector('.team-card__time');
    // Middle: members + progress
    const memberAvatars = el.querySelectorAll('.team-card__avatar, .team-card__member-avatar');
    const progressBar = el.querySelector('.team-card__progress, .q-linear-progress');
    const statusEl = el.querySelector('.team-card__status');
    const durationEl = el.querySelector('.team-card__duration');
    // Footer: buttons
    const footerBtns = Array.from(el.querySelectorAll('.team-card__footer button, .team-card__actions button, button')).map(b => ({
      text: (b.textContent || '').trim().slice(0, 30),
      cls: (typeof b.className === 'string' ? b.className : '').split(' ').filter(c => c.includes('cancel') || c.includes('retry') || c.includes('pause') || c.includes('resume') || c.includes('inject')).join(' '),
      visible: b.offsetParent !== null
    })).filter(b => b.cls || b.text);
    // Members / agent-cards inside
    const innerAgentCards = el.querySelectorAll('.agent-card');
    return {
      idx: i,
      cls: cls.slice(0, 200),
      rect: { w: Math.round(rect.width), h: Math.round(rect.height) },
      name: (nameEl?.textContent || '').trim().slice(0, 80),
      task: (taskEl?.textContent || '').trim().slice(0, 80),
      time: (timeEl?.textContent || '').trim().slice(0, 40),
      memberCount: memberAvatars.length,
      hasProgress: !!progressBar,
      status: (statusEl?.textContent || '').trim().slice(0, 40),
      duration: (durationEl?.textContent || '').trim().slice(0, 40),
      buttons: footerBtns.slice(0, 8),
      innerAgentCards: innerAgentCards.length
    };
  });

  // Inspect each agent-card
  out.agentCards = agentCards.map((el, i) => {
    const cls = el.className;
    const rect = el.getBoundingClientRect();
    const nameEl = el.querySelector('.agent-card__name');
    const statusBadge = el.querySelector('.agent-card__status-badge');
    const timeEl = el.querySelector('.agent-card__time');
    // Footer
    const footerBtns = Array.from(el.querySelectorAll('button')).map(b => ({
      text: (b.textContent || '').trim().slice(0, 30),
      cls: (typeof b.className === 'string' ? b.className : '').split(' ').filter(c => c.includes('cancel') || c.includes('retry') || c.includes('pause') || c.includes('resume') || c.includes('inject')).join(' '),
      visible: b.offsetParent !== null
    })).filter(b => b.cls || b.text);
    // Children
    const children = el.querySelectorAll('.thinking-block, .reply-block, .act-activity, .plan-block');
    const childKinds = Array.from(children).map(c => {
      const ccls = typeof c.className === 'string' ? c.className : '';
      if (ccls.includes('thinking-block')) return 'thinking';
      if (ccls.includes('reply-block')) return 'reply';
      if (ccls.includes('act-activity')) return 'action';
      if (ccls.includes('plan-block')) return 'plan';
      return '?';
    });
    return {
      idx: i,
      cls: cls.slice(0, 200),
      rect: { w: Math.round(rect.width), h: Math.round(rect.height) },
      name: (nameEl?.textContent || '').trim().slice(0, 80),
      status: (statusBadge?.textContent || '').trim().slice(0, 40),
      time: (timeEl?.textContent || '').trim().slice(0, 40),
      buttons: footerBtns.slice(0, 6),
      childCount: children.length,
      childKinds: childKinds
    };
  });

  // Plan block details
  const planBlocks = Array.from(document.querySelectorAll('.plan-block'));
  out.planBlocks = planBlocks.map((el, i) => {
    const steps = Array.from(el.querySelectorAll('.plan-block__step'));
    return {
      idx: i,
      cls: el.className.slice(0, 200),
      stepCount: steps.length,
      steps: steps.map(s => {
        const numEl = s.querySelector('.plan-block__step-num');
        const nameEl = s.querySelector('.plan-block__step-name');
        const statusEl = s.querySelector('.plan-block__step-status');
        return {
          num: (numEl?.textContent || '').trim(),
          name: (nameEl?.textContent || '').trim().slice(0, 60),
          status: (statusEl?.textContent || '').trim().slice(0, 40),
          cls: s.className.split(' ').filter(c => c.includes('--')).join(' ')
        };
      })
    };
  });

  return JSON.stringify(out, null, 2);
})()
