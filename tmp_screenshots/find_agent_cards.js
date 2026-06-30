(function() {
  const cards = Array.from(document.querySelectorAll('.agent-card')).slice(0, 5);
  const result = [];
  for (const card of cards) {
    const clickable = card.querySelector('[class*="chevron"], [class*="expand"], [class*="header"], button') || card;
    result.push({
      className: card.className,
      text: card.textContent.slice(0, 120).replace(/\s+/g, ' '),
      clickableTag: clickable.tagName,
      clickableClass: clickable.className,
      hasExpandedClass: /expanded/i.test(card.className)
    });
  }
  return JSON.stringify(result, null, 2);
})();
