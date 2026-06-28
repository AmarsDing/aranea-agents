JSON.stringify({
  actionBlocks: Array.from(document.querySelectorAll('.act-activity')).map(function(e, i) {
    var statusSpan = e.querySelector('.act-activity__status');
    var durationSpan = e.querySelector('.act-activity__duration');
    return {
      idx: i,
      rootClass: e.className,
      statusSpanClass: statusSpan ? statusSpan.className : 'NOT_FOUND',
      statusIcon: statusSpan ? statusSpan.textContent : 'NOT_FOUND',
      durationText: durationSpan ? durationSpan.textContent : 'NO_DURATION_SPAN',
      toolLabel: (e.querySelector('.act-activity__tool-label') || {}).textContent || ''
    };
  }).slice(-8)
}, null, 2)
