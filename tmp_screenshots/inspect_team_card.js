(function() {
  const cards = Array.from(document.querySelectorAll('.team-card'));
  const result = [];
  for (const card of cards) {
    const header = card.querySelector('.team-card__header, [class*="header"]');
    const body = card.querySelector('.team-card__body, [class*="body"]');
    const footer = card.querySelector('.team-card__footer, [class*="footer"]');
    const members = card.querySelectorAll('.agent-card, .team-card__member, [class*="member"]');
    result.push({
      className: card.className,
      textPreview: card.textContent.slice(0, 200).replace(/\s+/g, ' '),
      headerText: header ? header.textContent.slice(0, 200).replace(/\s+/g, ' ') : null,
      bodyText: body ? body.textContent.slice(0, 300).replace(/\s+/g, ' ') : null,
      footerText: footer ? footer.textContent.slice(0, 200).replace(/\s+/g, ' ') : null,
      memberCount: members.length,
      memberTexts: Array.from(members).map(m => m.textContent.slice(0, 150).replace(/\s+/g, ' ')),
      childList: Array.from(card.children).map(c => ({ tag: c.tagName, cls: c.className }))
    });
  }
  return JSON.stringify(result, null, 2);
})();
