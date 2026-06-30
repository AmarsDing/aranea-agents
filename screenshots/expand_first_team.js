(() => {
  // Find the first team-card (the one with 4 members, "## 任务概述")
  const cards = Array.from(document.querySelectorAll('.team-card'));
  const firstCard = cards.find(c => {
    const name = c.querySelector('.team-card__name')?.textContent || '';
    return name.startsWith('##');
  });
  if (!firstCard) return JSON.stringify({ error: 'first team card not found' });
  // Check if already expanded
  const isExpanded = firstCard.classList.contains('team-card--expanded');
  if (!isExpanded) {
    firstCard.querySelector('.team-card__header')?.click();
  }
  return JSON.stringify({ found: true, wasExpanded: isExpanded, nowClicked: !isExpanded });
})()
