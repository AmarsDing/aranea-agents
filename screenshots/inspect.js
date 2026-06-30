JSON.stringify({
  teamCards: Array.from(document.querySelectorAll('[class*="team-card"]')).map(el => ({
    classes: el.className.slice(0, 80),
    text: (el.textContent || '').slice(0, 80)
  })),
  agentCards: Array.from(document.querySelectorAll('[class*="agent-card"]')).map(el => {
    const detail = el.querySelector('[class*="detail"]');
    const name = el.querySelector('[class*="name"]');
    const status = el.querySelector('[class*="status-badge"]');
    return {
      classes: el.className.slice(0, 80),
      name: (name ? name.textContent : '').slice(0, 50),
      status: (status ? status.textContent : '').slice(0, 50),
      hasDetail: !!detail,
      detailChildrenCount: detail ? detail.children.length : 0,
      detailFirstChild: detail && detail.children[0] ? detail.children[0].className : ''
    };
  })
}, null, 2)
