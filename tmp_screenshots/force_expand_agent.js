(function() {
  const expandedTeam = document.querySelector('.team-card--expanded');
  if (!expandedTeam) return 'no expanded team';
  const agentCard = expandedTeam.querySelector('.agent-card');
  if (!agentCard) return 'no agent-card';
  agentCard.classList.add('agent-card--expanded');
  let detail = agentCard.querySelector('.agent-card__detail');
  if (!detail) {
    detail = document.createElement('div');
    detail.className = 'agent-card__detail';
    detail.innerHTML = '<div class="event-stream" style="min-height:120px;padding:8px;border:1px dashed #ccc;">模拟展开的 agent-card detail 内容</div>';
    agentCard.appendChild(detail);
  }
  detail.style.display = 'flex';
  // Scroll to see layout
  agentCard.scrollIntoView({ behavior: 'instant', block: 'start' });
  return 'force expanded agent-card, height=' + agentCard.offsetHeight;
})();
