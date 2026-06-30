(() => {
  const out = {};
  // 1. Overall page structure
  const teamCards = Array.from(document.querySelectorAll('.team-card, [class*="team-card"], [class*="TeamCard"]'));
  const agentCards = Array.from(document.querySelectorAll('.agent-card, [class*="agent-card"], [class*="AgentCard"]'));
  const graphCards = Array.from(document.querySelectorAll('.graph-stage, [class*="graph-stage"], [class*="GraphStage"]'));
  out.counts = { teamCards: teamCards.length, agentCards: agentCards.length, graphCards: graphCards.length };

  // 2. Helper: extract structure of a card
  function describeCard(el, label) {
    const cls = el.className;
    const headerText = (el.querySelector('.header, [class*="header"]')?.textContent || '').trim().slice(0, 200);
    const buttons = Array.from(el.querySelectorAll('button')).map(b => ({
      text: (b.textContent || '').trim().slice(0, 40),
      cls: b.className,
      visible: b.offsetParent !== null
    }));
    return { label, cls: typeof cls === 'string' ? cls.slice(0, 200) : '', headerText, buttons };
  }

  // 3. Try multiple selectors for team-card
  out.teamCards = teamCards.slice(0, 5).map((el, i) => describeCard(el, `team-${i}`));
  out.agentCards = agentCards.slice(0, 10).map((el, i) => describeCard(el, `agent-${i}`));

  // 4. Find all activity blocks in order
  const allBlocks = Array.from(document.querySelectorAll('[class*="activity"], [class*="block"], [class*="thinking"], [class*="reply"], [class*="action"], [class*="plan"]'));
  out.blockSummary = allBlocks.slice(0, 50).map(el => {
    const cls = typeof el.className === 'string' ? el.className : '';
    return cls.split(' ').filter(c => c.length > 0).slice(0, 3).join(' ');
  });

  // 5. Find plan blocks
  const planBlocks = Array.from(document.querySelectorAll('[class*="plan"], [class*="Plan"]'));
  out.planBlocks = planBlocks.slice(0, 5).map((el, i) => {
    const text = (el.textContent || '').trim().slice(0, 300);
    const steps = Array.from(el.querySelectorAll('[class*="step"], [class*="item"], li')).map(s => ({
      text: (s.textContent || '').trim().slice(0, 80),
      cls: typeof s.className === 'string' ? s.className.slice(0, 100) : ''
    }));
    return { idx: i, text, stepCount: steps.length, steps: steps.slice(0, 10) };
  });

  return JSON.stringify(out, null, 2);
})()
