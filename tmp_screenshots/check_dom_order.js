// Check DOM order of activity blocks in the chat stream
(() => {
  const stream = document.querySelector('.event-stream') || document.querySelector('[class*="event-stream"]') || document.querySelector('[class*="activity"]');
  if (!stream) {
    return JSON.stringify({ error: 'no event-stream container found' });
  }

  // Find all direct block children
  const blocks = [];
  const walk = (el, depth = 0) => {
    const cls = el.className || '';
    const tag = el.tagName || '';
    // Identify block type by class names or data attributes
    let type = 'unknown';
    if (/thinking/i.test(cls)) type = 'thinking';
    else if (/action-block/i.test(cls)) type = 'action';
    else if (/reply-block/i.test(cls)) type = 'reply';
    else if (/plan-block/i.test(cls)) type = 'plan';
    else if (/graph-stage/i.test(cls)) type = 'graph_stage';
    else if (/team-card/i.test(cls)) type = 'team-card';
    else if (/agent-card/i.test(cls)) type = 'agent-card';
    else if (/user-message/i.test(cls)) type = 'task';
    else if (/notice-block/i.test(cls)) type = 'notice';
    else if (/error-block/i.test(cls)) type = 'error';

    const text = (el.textContent || '').slice(0, 80).replace(/\s+/g, ' ').trim();
    if (type !== 'unknown' || text) {
      blocks.push({ depth, type, tag, cls: String(cls).slice(0, 100), text });
    }

    // Recurse into children but only first 3 levels
    if (depth < 3) {
      for (const child of el.children) {
        walk(child, depth + 1);
      }
    }
  };

  walk(stream, 0);

  return JSON.stringify(blocks, null, 2);
})();
