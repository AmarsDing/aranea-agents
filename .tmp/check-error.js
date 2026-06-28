(() => {
  // 检查是否有错误提示、loading 状态、或空状态
  const errorEls = document.querySelectorAll('[class*="error"], [class*="alert"], [class*="warning"], .q-banner, .q-notification');
  const loadingEls = document.querySelectorAll('[class*="loading"], [class*="spinner"], .q-spinner, [role="progressbar"]');
  const emptyEls = document.querySelectorAll('[class*="empty"], [class*="placeholder"], [class*="no-data"]');

  // 检查 chat-page 的主要内容区
  const chatMain = document.querySelector('.chat-page__main, .chat-workspace-shell__main, .q-page-container');

  // 查找所有带有 text 内容的可见元素（最近添加的）
  const allDivs = document.querySelectorAll('div');
  const recentText = [];
  allDivs.forEach((d) => {
    const text = (d.textContent || '').trim();
    if (text.length > 20 && text.length < 200 && d.children.length < 5) {
      const rect = d.getBoundingClientRect();
      if (rect.top > 400 && rect.top < 1200 && rect.width > 200) {
        recentText.push({
          text: text.slice(0, 150),
          cls: (d.className || '').slice(0, 80),
          top: Math.round(rect.top),
        });
      }
    }
  });

  return JSON.stringify({
    errors: Array.from(errorEls).slice(0, 3).map((e) => ({
      cls: (e.className || '').slice(0, 80),
      text: (e.textContent || '').trim().slice(0, 150),
    })),
    loading: Array.from(loadingEls).slice(0, 3).map((e) => ({
      cls: (e.className || '').slice(0, 80),
    })),
    empty: Array.from(emptyEls).slice(0, 3).map((e) => ({
      cls: (e.className || '').slice(0, 80),
      text: (e.textContent || '').trim().slice(0, 100),
    })),
    recentText: recentText.slice(0, 5),
  }, null, 2);
})();
