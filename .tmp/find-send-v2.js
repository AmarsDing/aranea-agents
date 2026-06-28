(() => {
  const textarea = document.querySelector('textarea');
  const tr = textarea.getBoundingClientRect();
  const buttons = Array.from(document.querySelectorAll('button'));
  // Find buttons below the textarea (same row or just below)
  const nearby = buttons.filter((b) => {
    const r = b.getBoundingClientRect();
    return r.top > tr.top - 50 && r.top < tr.top + 100 && r.left > tr.left;
  });
  return JSON.stringify({
    textareaTop: tr.top,
    textareaLeft: tr.left,
    nearbyButtons: nearby.map((b) => ({
      text: (b.textContent || '').trim().slice(0, 50),
      ariaLabel: b.getAttribute('aria-label') || '',
      title: b.getAttribute('title') || '',
      disabled: b.disabled,
      cls: (b.className || '').slice(0, 80),
      icon: b.querySelector('i, .q-icon')?.textContent || '',
      rect: { top: Math.round(b.getBoundingClientRect().top), left: Math.round(b.getBoundingClientRect().left) },
    })),
  }, null, 2);
})();
