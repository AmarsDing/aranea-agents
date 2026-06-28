(() => {
  const textarea = document.querySelector('textarea');
  if (!textarea) return JSON.stringify({ error: 'no textarea found' });
  const rect = textarea.getBoundingClientRect();
  const parent = textarea.closest('form, .q-form, [class*="composer"], [class*="input"], [class*="chat"]');
  const parentInfo = parent ? {
    tag: parent.tagName,
    cls: (parent.className || '').slice(0, 200),
    visible: parent.offsetParent !== null,
  } : null;
  // Check all buttons on page
  const allButtons = Array.from(document.querySelectorAll('button'));
  const visibleButtons = allButtons.filter((b) => b.offsetParent !== null && b.getBoundingClientRect().top > 0);
  return JSON.stringify({
    textarea: {
      value: textarea.value.slice(0, 100),
      rect: { top: rect.top, left: rect.left, width: rect.width, height: rect.height },
      visible: textarea.offsetParent !== null,
      disabled: textarea.disabled,
    },
    parent: parentInfo,
    totalButtons: allButtons.length,
    visibleButtons: visibleButtons.length,
    visibleButtonSample: visibleButtons.slice(0, 8).map((b) => ({
      text: (b.textContent || '').trim().slice(0, 30),
      ariaLabel: b.getAttribute('aria-label') || '',
      title: b.getAttribute('title') || '',
      disabled: b.disabled,
      cls: (b.className || '').slice(0, 60),
      rect: { top: Math.round(b.getBoundingClientRect().top), left: Math.round(b.getBoundingClientRect().left) },
    })),
  }, null, 2);
})();
