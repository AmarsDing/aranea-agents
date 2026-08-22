(() => {
  const res = {};
  const ta = document.querySelector('textarea');
  res.taValue = ta ? ta.value : null;
  const runnerEl = document.querySelector('.chat-message-header__runner');
  res.runnerActive = !!runnerEl;
  res.runnerText = runnerEl ? runnerEl.textContent : null;
  res.hasContextItem = !!document.querySelector('.spirit-status-bar .q-icon[name="data_usage"]');
  res.statusBarText = (document.querySelector('.spirit-status-bar') || {}).textContent || null;
  return JSON.stringify(res);
})()
