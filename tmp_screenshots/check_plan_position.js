// Check plan-block position and parent in the DOM
(() => {
  const plans = document.querySelectorAll('.plan-block');
  const result = [];
  plans.forEach((p, i) => {
    // Walk up to find the parent activity block
    let parent = p.parentElement;
    const parents = [];
    while (parent && parent !== document.body) {
      const cls = parent.className || '';
      if (typeof cls === 'string' && (cls.includes('activity') || cls.includes('task') || cls.includes('stream') || cls.includes('card'))) {
        parents.push(cls.split(' ').slice(0, 2).join(' '));
      }
      parent = parent.parentElement;
    }
    result.push({
      idx: i,
      class: p.className,
      text: (p.textContent || '').trim().slice(0, 60),
      parents: parents.slice(0, 5),
    });
  });
  // Also check the overall structure of root-level blocks
  const stream = document.querySelector('.activity-stream') || document.querySelector('[class*="activity-stream"]');
  let rootEl = stream || document.body;
  const children = [];
  Array.from(rootEl.children).forEach((c, i) => {
    const cls = c.className || '';
    children.push({
      idx: i,
      class: typeof cls === 'string' ? cls.split(' ').slice(0, 2).join(' ') : '',
      text: (c.textContent || '').trim().slice(0, 50),
    });
  });
  return { plans: result, rootChildren: children };
})();
