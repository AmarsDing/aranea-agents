// Try to find Pinia store or Vue instance to inspect activity data
(() => {
  // Try to find team_stage activity data via Vue devtools or component data
  const teamCards = Array.from(document.querySelectorAll('.team-card'));
  const results = [];
  
  for (const card of teamCards) {
    // Try to get Vue component instance
    const vueInstance = card.__vueParentComponent || card.__vue__;
    if (vueInstance) {
      const props = vueInstance.props || {};
      const activity = props.activity || {};
      results.push({
        activityId: activity.id,
        status: activity.status,
        kind: activity.kind,
        progressPct: activity.progressPct,
        members: (activity.members || []).map(m => ({ agentKey: m.agentKey, agentName: m.agentName, status: m.status })),
        meta: activity.meta
      });
    } else {
      results.push({ error: 'No Vue instance found' });
    }
  }
  
  return JSON.stringify({ teamCardActivities: results }, null, 2);
})()
