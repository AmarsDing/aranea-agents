(function() {
  const plan = document.querySelector('.plan-block');
  if (!plan) return 'no plan-block';
  // Find the activity data in the timeline store
  const vueApp = document.querySelector('#q-app') || document.body;
  const keys = Object.keys(vueApp);
  const vueKey = keys.find(k => k.startsWith('__vue'));
  let storeInfo = 'no store access';
  if (vueKey) {
    const app = vueApp[vueKey];
    // Try to access pinia stores through app
    const stores = app?.appContext?.config?.globalProperties?.$pinia?.state?.value;
    if (stores) {
      const chatStore = stores.chat || stores.activityTimeline;
      if (chatStore) {
        const activities = chatStore.activities || chatStore.activityMap || chatStore.timeline;
        storeInfo = 'store type=' + typeof activities;
        if (activities && typeof activities === 'object') {
          const planActivity = Object.values(activities).find(a => a && a.kind === 'plan');
          if (planActivity) {
            return JSON.stringify({ planActivity }, null, 2);
          }
        }
      }
    }
  }
  // Fallback: read DOM text
  return JSON.stringify({
    text: plan.textContent.slice(0, 300).replace(/\s+/g, ' '),
    classes: plan.className
  }, null, 2);
})();
