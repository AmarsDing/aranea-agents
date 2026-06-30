JSON.stringify({
  teamCards: Array.from(document.querySelectorAll('[class*="team"], [data-kind="team_stage"]')).map(el => ({
    cls: el.className,
    tag: el.tagName,
    html: el.outerHTML.slice(0, 800)
  })),
  planBlocks: Array.from(document.querySelectorAll('[class*="plan"], [data-kind="plan"]')).map(el => ({
    cls: el.className,
    html: el.outerHTML.slice(0, 500)
  })),
  graphStages: Array.from(document.querySelectorAll('[class*="graph"], [data-kind="graph_stage"]')).map(el => ({
    cls: el.className,
    html: el.outerHTML.slice(0, 300)
  })),
  agentCards: Array.from(document.querySelectorAll('[class*="agent-card"], [data-kind="session"]')).map(el => ({
    cls: el.className,
    html: el.outerHTML.slice(0, 300)
  })),
  allActivityKinds: Array.from(document.querySelectorAll('[data-kind]')).map(el => el.getAttribute('data-kind'))
}, null, 2)
