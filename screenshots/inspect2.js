(() => {
  const cards = Array.from(document.querySelectorAll('.agent-card'));
  return JSON.stringify(cards.map(el => {
    const detail = el.querySelector('.agent-card__detail');
    const name = el.querySelector('.agent-card__name')?.textContent || '';
    const status = el.querySelector('.agent-card__status-badge')?.textContent || '';
    return {
      name: name.slice(0, 40),
      status: status.trim().slice(0, 40),
      expanded: !!detail,
      childCount: detail ? detail.children.length : 0,
      childTypes: detail ? Array.from(detail.children).map(c => c.className.slice(0, 50)) : []
    };
  }), null, 2);
})()
