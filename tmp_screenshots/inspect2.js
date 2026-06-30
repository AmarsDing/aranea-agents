JSON.stringify({
  // Check team-card progress bar widths
  progressBars: Array.from(document.querySelectorAll('.team-card')).map((card, i) => {
    const fill = card.querySelector('.team-card__bar-fill');
    const status = card.querySelector('.team-card__status');
    const members = Array.from(card.querySelectorAll('.team-card__member')).map(m => m.className);
    const cardStatus = card.className;
    const row = card.querySelector('.team-card__row');
    const rowStyle = row ? window.getComputedStyle(row) : null;
    const headerStyle = card.querySelector('.team-card__header') ? window.getComputedStyle(card.querySelector('.team-card__header')) : null;
    const bodyStyle = card.querySelector('.team-card__body') ? window.getComputedStyle(card.querySelector('.team-card__body')) : null;
    const footerStyle = card.querySelector('.team-card__footer') ? window.getComputedStyle(card.querySelector('.team-card__footer')) : null;
    return {
      idx: i,
      cardStatus,
      progressFillWidth: fill ? fill.style.width : 'no fill',
      statusText: status ? status.textContent.trim() : 'no status',
      memberCount: members.length,
      memberClasses: members,
      rowDisplay: rowStyle ? rowStyle.display : 'no row',
      rowFlexDirection: rowStyle ? rowStyle.flexDirection : 'no row',
      headerFlexBasis: headerStyle ? headerStyle.flexBasis : 'no header',
      bodyFlexBasis: bodyStyle ? bodyStyle.flexBasis : 'no body',
      footerFlexBasis: footerStyle ? footerStyle.flexBasis : 'no footer',
      // Check if expanded
      isExpanded: card.classList.contains('team-card--expanded'),
      // Check for child activity stream
      hasChildStream: !!card.querySelector('.team-card__expanded, .event-stream')
    };
  }),
  // Check plan-block state
  planBlocks: Array.from(document.querySelectorAll('.plan-block')).map(b => {
    const collapsed = b.classList.contains('plan-block--collapsed');
    const steps = Array.from(b.querySelectorAll('.plan-block__step')).length;
    const summary = b.querySelector('.plan-block__summary')?.textContent.trim();
    const count = b.querySelector('.plan-block__count')?.textContent.trim();
    const status = b.className;
    return { collapsed, steps, summary, count, status };
  }),
  // Check graph-stage
  graphStages: Array.from(document.querySelectorAll('.graph-stage-block')).map(g => {
    const nodes = Array.from(g.querySelectorAll('.graph-stage-block__node')).map(n => ({
      cls: n.className,
      label: n.querySelector('.graph-stage-block__node-label')?.textContent.trim()
    }));
    return { nodes, status: g.className };
  }),
  // Check agent-cards
  agentCards: Array.from(document.querySelectorAll('.agent-card')).map(a => ({
    status: a.className,
    name: a.querySelector('.agent-card__name')?.textContent.trim(),
    isExpanded: a.classList.contains('agent-card--expanded')
  })),
  // Check thinking/action blocks default state
  thinkingBlocks: Array.from(document.querySelectorAll('.thinking-block, [class*="ThinkingBlock"]')).length,
  actionBlocks: Array.from(document.querySelectorAll('.action-block, [class*="ActionBlock"]')).length
}, null, 2)
