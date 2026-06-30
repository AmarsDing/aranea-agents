// Inspect DOM order of activity blocks
(() => {
  const blocks = document.querySelectorAll('[class*="plan-block"], [class*="graph-stage"], [class*="team-card"], [class*="agent-card"], [class*="thinking-block"], [class*="reply-block"], [class*="action-block"], [class*="user-message"], [class*="conclusion"]');
  const result = [];
  blocks.forEach((b, i) => {
    const cls = b.className || '';
    const classStr = typeof cls === 'string' ? cls : '';
    result.push({
      idx: i,
      class: classStr.split(' ').slice(0, 2).join(' '),
      text: (b.textContent || '').trim().slice(0, 60),
    });
  });
  return { total: result.length, blocks: result };
})();
