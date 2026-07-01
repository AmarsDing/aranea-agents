(function() {
  const graph = document.querySelectorAll('.graph-stage-block');
  const team = document.querySelectorAll('.team-card');
  const agent = document.querySelectorAll('.agent-card');
  return JSON.stringify({
    graphStages: graph.length,
    teamCards: team.length,
    agentCards: agent.length,
    graphTexts: Array.from(graph).map(g => g.textContent.slice(0, 80)),
    teamTexts: Array.from(team).map(t => t.textContent.slice(0, 80)),
    agentTexts: Array.from(agent).map(a => a.textContent.slice(0, 80))
  }, null, 2);
})();
