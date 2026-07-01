(function() {
  const container = document.querySelector('.event-stream__children');
  if (!container) return 'no event-stream__children';
  const blocks = [];
  const walk = (el, depth = 0) => {
    if (depth > 3) return;
    const cls = el.className || '';
    let type = 'unknown';
    if (/user-message/i.test(cls)) type = 'task';
    else if (/thinking/i.test(cls)) type = 'thinking';
    else if (/action/i.test(cls)) type = 'action';
    else if (/reply/i.test(cls)) type = 'reply';
    else if (/plan/i.test(cls)) type = 'plan';
    else if (/graph-stage/i.test(cls)) type = 'graph_stage';
    else if (/team-card/i.test(cls)) type = 'team-card';
    else if (/agent-card/i.test(cls)) type = 'agent-card';
    if (type !== 'unknown' || depth === 0) {
      blocks.push({ depth, type, cls: cls.slice(0, 100), text: el.textContent.slice(0, 80).replace(/\s+/g, ' ') });
    }
    for (const child of el.children) walk(child, depth + 1);
  };
  walk(container, 0);
  return JSON.stringify(blocks, null, 2);
})();
