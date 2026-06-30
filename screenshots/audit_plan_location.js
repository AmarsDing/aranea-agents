// Find all plan blocks and their exact DOM location
(() => {
  const plans = document.querySelectorAll('.plan-block');
  const result = [];
  for (let i = 0; i < plans.length; i++) {
    const p = plans[i];
    // Get the first step name for identification
    const firstStep = p.querySelector('.plan-block__step-name')?.textContent?.trim() || '?';

    // Walk up to find the closest activity container
    let container = '';
    let el = p.parentElement;
    while (el) {
      const cls = el.className || '';
      if (typeof cls === 'string') {
        if (cls.includes('user-message')) { container = 'inside-user-message-bubble'; break; }
        if (cls.includes('event-stream') && !cls.includes('nested')) { container = 'root-event-stream'; break; }
        if (cls.includes('event-stream')) { container = 'nested-event-stream'; break; }
        if (cls.includes('team-card')) { container = 'inside-team-card'; break; }
        if (cls.includes('agent-card')) { container = 'inside-agent-card'; break; }
      }
      el = el.parentElement;
    }

    // Also check the plan's ID if available
    const planId = p.dataset?.activityId || p.closest('[data-activity-id]')?.dataset?.activityId || '?';

    result.push({ index: i, firstStep, container, planId });
  }
  return JSON.stringify(result, null, 2);
})();
