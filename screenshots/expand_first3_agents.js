(() => {
  const cards = Array.from(document.querySelectorAll('.agent-card'));
  // Expand the first 3 (部门主管-架构, 写作助手, 研发助手 from first team)
  const result = [];
  for (let i = 0; i < Math.min(3, cards.length); i++) {
    const card = cards[i];
    const name = card.querySelector('.agent-card__name')?.textContent || '';
    const isExpanded = card.querySelector('.agent-card__detail');
    if (!isExpanded) {
      card.querySelector('.agent-card__header')?.click();
    }
    result.push({ idx: i, name: name.slice(0, 30), clicked: !isExpanded });
  }
  return JSON.stringify(result);
})()
