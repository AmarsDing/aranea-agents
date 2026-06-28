(() => {
  const es = document.querySelector('.event-stream');
  const main = document.querySelector('main, .q-page, [class*="chat"]');
  return JSON.stringify({
    url: location.href,
    eventStreamExists: !!es,
    eventStreamChildren: es?.children?.length || 0,
    mainHTML: main?.innerHTML?.slice(0, 500) || 'no main',
    bodyText: document.body.innerText.slice(0, 300),
  });
})();
