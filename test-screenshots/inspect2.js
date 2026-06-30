JSON.stringify({
  teamDetailInner: (document.querySelector('.team-card__detail') || {}).innerHTML || 'EMPTY',
  teamDetailChildren: (document.querySelector('.team-card__detail') || {}).children?.length || 0,
  eventStreamChildren: document.querySelectorAll('.event-stream__children').length,
  agentClasses: Array.from(new Set(Array.from(document.querySelectorAll('[class*="agent"]')).flatMap(el => Array.from(el.classList)).filter(c => c.includes('agent')))).slice(0, 20),
  // Check for any session-related elements
  sessionEls: Array.from(document.querySelectorAll('[class*="session"]')).slice(0, 5).map(el => ({tag: el.tagName, class: el.className, text: (el.textContent || '').slice(0, 80)}))
})
