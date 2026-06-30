// Get the spirit session ID from the current page
(() => {
  // Try to find spirit session ID from URL or store
  const url = window.location.href;
  const match = url.match(/session[=/]([a-f0-9-]+)/i);
  
  // Try to find via Vue instance
  const sessionList = document.querySelectorAll('[class*="session-item"], [class*="session-list"] li');
  const activeSession = Array.from(sessionList).find(li => li.classList.contains('active') || li.querySelector('.q-item--active'));
  
  // Try to get from the page's Pinia store
  const app = document.querySelector('#q-app');
  if (app && app.__vue_app__) {
    const stores = app.__vue_app__._instance?.appContext?.config?.globalProperties?.$pinia?._s;
    if (stores) {
      const chatStore = stores.get('chat') || stores.get('chatStore');
      if (chatStore) {
        return JSON.stringify({
          currentSessionId: chatStore.currentSessionId || chatStore.currentSession?.id,
          spiritSessionId: chatStore.spiritSessionId,
          url,
          storeKeys: Array.from(stores.keys())
        });
      }
      return JSON.stringify({ storeKeys: Array.from(stores.keys()), url });
    }
  }
  return JSON.stringify({ url, error: 'no store found' });
})()
