(() => {
  const teamCards = document.querySelectorAll('.team-card');
  if (teamCards.length > 0) {
    const header = teamCards[0].querySelector('.team-card__header');
    if (header) {
      header.click();
      return JSON.stringify({ clicked: true, teamCardCount: teamCards.length });
    }
  }
  return JSON.stringify({ clicked: false, teamCardCount: teamCards ? teamCards.length : 0 });
})()
