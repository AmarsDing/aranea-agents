(() => {
  const buttons = document.querySelectorAll('button');
  const sendButtons = [];
  buttons.forEach((b) => {
    const text = (b.textContent || '').trim();
    const ariaLabel = b.getAttribute('aria-label') || '';
    const title = b.getAttribute('title') || '';
    const cls = b.className || '';
    if (text.includes('发送') || ariaLabel.includes('发送') || title.includes('发送') ||
        text.includes('send') || ariaLabel.toLowerCase().includes('send') ||
        cls.includes('send')) {
      sendButtons.push({
        text: text.slice(0, 50),
        ariaLabel,
        title,
        cls: cls.slice(0, 100),
        disabled: b.disabled,
        rect: b.getBoundingClientRect(),
      });
    }
  });
  // Also find buttons near the textarea
  const textarea = document.querySelector('textarea');
  const nearbyButtons = [];
  if (textarea) {
    const tr = textarea.getBoundingClientRect();
    buttons.forEach((b) => {
      const br = b.getBoundingClientRect();
      const dist = Math.abs(br.top - tr.top) + Math.abs(br.left - tr.left);
      if (dist < 300 && br.top > 0) {
        nearbyButtons.push({
          text: (b.textContent || '').trim().slice(0, 30),
          ariaLabel: b.getAttribute('aria-label') || '',
          disabled: b.disabled,
          dist: Math.round(dist),
        });
      }
    });
  }
  return JSON.stringify({ sendButtons, nearbyButtons: nearbyButtons.slice(0, 10) }, null, 2);
})();
