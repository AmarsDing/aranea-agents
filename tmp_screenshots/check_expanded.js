JSON.stringify({
  expanded: Array.from(document.querySelectorAll('.team-card')).map((c, i) => ({
    idx: i,
    isExpanded: c.classList.contains('team-card--expanded'),
    hasChildStream: !!c.querySelector('.event-stream, .team-card__expanded'),
    childActivities: c.querySelector('.event-stream') ? c.querySelector('.event-stream').children.length : 0
  }))
}, null, 2)
