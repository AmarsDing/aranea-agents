JSON.stringify({
  graphBlocks: document.querySelectorAll('.graph-stage-block').length,
  graphNodes: document.querySelectorAll('.graph-stage-block__node, .graph-node, .graph-stage-block__nodes > *').length,
  planBlocksDetail: Array.from(document.querySelectorAll('.plan-block')).slice(0, 3).map(el => ({
    title: el.querySelector('.plan-block__title, h3, h4')?.textContent?.trim().slice(0, 40),
    stepCount: el.querySelectorAll('.plan-block__step, .plan-step, .plan-block__steps > *').length,
    hasStepsContainer: !!el.querySelector('.plan-block__steps'),
    innerHTML: el.innerHTML.slice(0, 200)
  })),
  sidebarTeams: document.querySelectorAll('.team-card, .sidebar-team, [class*="team"]').length,
  unifiedPanel: document.querySelectorAll('.unified-panel, [class*="unified-panel"]').length,
  panelSections: document.querySelectorAll('.panel-section, [class*="panel-section"]').length
})
