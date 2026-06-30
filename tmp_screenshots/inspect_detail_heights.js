(function() {
  const teamCard = document.querySelector('.team-card--expanded');
  if (!teamCard) return 'no expanded team-card';

  const detail = teamCard.querySelector('.team-card__detail');
  const row = teamCard.querySelector('.team-card__row');
  const styles = {
    teamCardHeight: teamCard.offsetHeight,
    teamCardRect: teamCard.getBoundingClientRect(),
    rowHeight: row ? row.offsetHeight : null,
    rowRect: row ? row.getBoundingClientRect() : null,
    detailHeight: detail ? detail.offsetHeight : null,
    detailRect: detail ? detail.getBoundingClientRect() : null,
    detailComputed: detail ? {
      display: getComputedStyle(detail).display,
      position: getComputedStyle(detail).position,
      overflow: getComputedStyle(detail).overflow,
      marginTop: getComputedStyle(detail).marginTop,
      paddingTop: getComputedStyle(detail).paddingTop,
      borderTop: getComputedStyle(detail).borderTop
    } : null
  };

  // Check child agent cards in detail
  const agentCards = detail ? Array.from(detail.querySelectorAll('.agent-card')).map(c => ({
    className: c.className,
    rect: c.getBoundingClientRect(),
    offsetHeight: c.offsetHeight
  })) : [];

  return JSON.stringify({ styles, agentCards }, null, 2);
})();
