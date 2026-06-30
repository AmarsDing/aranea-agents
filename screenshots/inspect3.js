(() => {
  const cards = Array.from(document.querySelectorAll('.agent-card'));
  return JSON.stringify(cards.map((el, idx) => {
    const detail = el.querySelector('.agent-card__detail');
    const eventStream = detail?.querySelector('.event-stream');
    const children = eventStream ? Array.from(eventStream.children) : [];
    return {
      idx,
      name: (el.querySelector('.agent-card__name')?.textContent || '').slice(0, 30),
      eventStreamChildren: children.length,
      childClasses: children.map(c => c.className.slice(0, 60)),
      childTexts: children.map(c => (c.textContent || '').slice(0, 50))
    };
  }), null, 2);
})()
