(() => {
  // 1. Expand all TeamCard headers
  const teamHeaders = document.querySelectorAll('.team-card__header, [class*="team-card-header"]');
  teamHeaders.forEach(h => {
    try { h.click(); } catch(e) {}
  });

  // Wait a bit then audit
  setTimeout(() => {
    // 2. Audit TeamCard and AgentCard statuses
    const result = { teams: [], agents: [] };

    // Find all team cards
    const teamCards = document.querySelectorAll('[class*="team-card"], [data-team-id]');
    teamCards.forEach((card, i) => {
      const text = card.innerText || '';
      const statusEl = card.querySelector('[class*="status"], .q-chip, .badge');
      result.teams.push({
        index: i,
        text: text.substring(0, 200),
        status: statusEl ? statusEl.innerText : '(no status el)'
      });
    });

    // Find all agent cards
    const agentCards = document.querySelectorAll('[class*="agent-card"], [data-agent-key]');
    agentCards.forEach((card, i) => {
      const text = card.innerText || '';
      const statusEl = card.querySelector('[class*="status"], .q-chip, .badge');
      result.agents.push({
        index: i,
        text: text.substring(0, 200),
        status: statusEl ? statusEl.innerText : '(no status el)'
      });
    });

    console.log('AUDIT_RESULT:', JSON.stringify(result, null, 2));
  }, 1500);
})();
