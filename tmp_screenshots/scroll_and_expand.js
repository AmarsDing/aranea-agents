(function() {
  const graph = document.querySelector('.graph-stage-block');
  if (graph) graph.scrollIntoView({ behavior: 'instant', block: 'start' });
  const cards = document.querySelectorAll('.team-card');
  if (!cards.length) return 'no team-cards';
  // Expand the last team-card in graph_stage (综合报告汇总 is usually last)
  const card = cards[cards.length - 1];
  const header = card.querySelector('.team-card__header') || card;
  const evt = new MouseEvent('click', { bubbles: true, cancelable: true, view: window });
  header.dispatchEvent(evt);
  return 'clicked card ' + (cards.length - 1) + ', classes=' + card.className + ', y=' + card.getBoundingClientRect().top;
})();
