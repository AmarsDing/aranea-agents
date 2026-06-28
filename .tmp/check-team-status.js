JSON.stringify({
  teamsStatus: Array.from(document.querySelectorAll('.team-stage-block')).map(el => ({
    name: el.querySelector('.team-stage-block__name')?.textContent?.slice(0, 30),
    status: el.querySelector('.team-stage-block__status-badge')?.textContent?.trim().slice(0, 20),
    classes: el.className.split(' ').filter(c => c.startsWith('team-stage-block--')).join(',')
  })),
  piniaTeams: (function(){
    try {
      const app = document.querySelector('#q-app, #app, [id]')?.__vue_app__;
      if (!app) return 'no vue app';
      const stores = app.config.globalProperties.$pinia?._s;
      if (!stores) return 'no pinia stores';
      const spiritStore = stores.get('spirit');
      if (!spiritStore) return 'no spirit store';
      return spiritStore.teams.map(t => ({
        id: t.id?.slice(0, 8),
        name: t.teamName?.slice(0, 30),
        status: t.status,
        progressPct: t.progressPct,
        memberCount: t.members?.length || 0
      }));
    } catch(e) { return 'error: ' + e.message; }
  })()
})
