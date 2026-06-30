(() => {
  const out = {};

  // Precise class match
  function hasExactClass(el, name) {
    if (typeof el.className !== 'string') return false;
    return el.className.split(/\s+/).some(c => c === name || c.startsWith(name + '--'));
  }

  const teamCards = Array.from(document.querySelectorAll('div, section, article')).filter(el => hasExactClass(el, 'team-card'));
  const agentCards = Array.from(document.querySelectorAll('div, section, article')).filter(el => hasExactClass(el, 'agent-card'));

  out.teamCards = teamCards.map((el, i) => {
    const nameEl = el.querySelector('.team-card__name');
    const taskEl = el.querySelector('.team-card__task');
    return {
      idx: i,
      cls: el.className,
      name: (nameEl?.textContent || '').trim().slice(0, 120),
      task: (taskEl?.textContent || '').trim().slice(0, 120)
    };
  });

  out.agentCards = agentCards.map((el, i) => {
    const nameEl = el.querySelector('.agent-card__name');
    const statusBadge = el.querySelector('.agent-card__status-badge');
    // Check for action buttons (precise selectors)
    const pauseBtn = el.querySelector('.agent-card__action--pause');
    const cancelBtn = el.querySelector('.agent-card__action--cancel');
    const retryBtn = el.querySelector('.agent-card__action--retry');
    const injectInput = el.querySelector('.agent-card__inject-input');
    return {
      idx: i,
      cls: el.className,
      name: (nameEl?.textContent || '').trim().slice(0, 80),
      status: (statusBadge?.textContent || '').trim().slice(0, 40),
      buttons: {
        pause: !!pauseBtn,
        cancel: !!cancelBtn,
        retry: !!retryBtn,
        inject: !!injectInput
      }
    };
  });

  return JSON.stringify(out, null, 2);
})()
