JSON.stringify((function(){
  try {
    const app = document.querySelector('#q-app')?.__vue_app__;
    if (!app) return {error: 'no vue app'};
    const pinia = app.config.globalProperties.$pinia;
    if (!pinia) return {error: 'no pinia'};
    const store = pinia._s?.get('spiritTeam');
    if (!store) return {error: 'no spiritTeam store', stores: Array.from(pinia._s?.keys() || [])};
    return {
      teamsCount: store.teams?.length || 0,
      teams: (store.teams || []).map(t => ({
        id: t.id?.slice(0, 8),
        name: t.teamName?.slice(0, 30),
        status: t.status,
        progressPct: t.progressPct,
        memberCount: t.members?.length || 0
      }))
    };
  } catch(e) { return {error: e.message, stack: e.stack}; }
})())
