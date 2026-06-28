(() => {
  const textarea = document.querySelector('textarea');
  const tr = textarea.getBoundingClientRect();
  const buttons = Array.from(document.querySelectorAll('button'));
  const nearby = buttons.filter((b) => {
    const r = b.getBoundingClientRect();
    return r.top > tr.top - 50 && r.top < tr.top + 100;
  });
  return JSON.stringify({
    textareaValue: textarea.value.slice(0, 60),
    nearbyButtons: nearby.map((b) => {
      const r = b.getBoundingClientRect();
      const icon = b.querySelector('i, .q-icon');
      return {
        text: (b.textContent || '').trim().slice(0, 30),
        ariaLabel: b.getAttribute('aria-label') || '',
        disabled: b.disabled,
        iconText: icon?.textContent || '',
        iconClass: icon?.className || '',
        rect: { top: Math.round(r.top), left: Math.round(r.left) },
      };
    }),
  }, null, 2);
})();
