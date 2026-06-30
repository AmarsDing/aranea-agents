(function() {
  const agentCard = document.querySelector('.agent-card--expanded') || document.querySelector('.agent-card');
  const teamCard = document.querySelectorAll('.team-card')[1]; // 代码分析团队
  if (!agentCard || !teamCard) return 'missing cards';

  const aRect = agentCard.getBoundingClientRect();
  const tRect = teamCard.getBoundingClientRect();

  return JSON.stringify({
    agentCard: {
      className: agentCard.className,
      top: aRect.top,
      bottom: aRect.bottom,
      height: aRect.height,
      zIndex: getComputedStyle(agentCard).zIndex,
      position: getComputedStyle(agentCard).position
    },
    teamCard: {
      className: teamCard.className,
      top: tRect.top,
      bottom: tRect.bottom,
      height: tRect.height,
      zIndex: getComputedStyle(teamCard).zIndex,
      position: getComputedStyle(teamCard).position
    },
    overlap: aRect.bottom > tRect.top
  }, null, 2);
})();
