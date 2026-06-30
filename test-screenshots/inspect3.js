var detail = document.querySelector('.team-card__detail');
JSON.stringify({
  exists: !!detail,
  childrenCount: detail ? detail.children.length : 0,
  innerHTML: detail ? detail.innerHTML.slice(0, 2000) : 'NO DETAIL ELEMENT',
  textContent: detail ? (detail.textContent || '').trim().slice(0, 200) : 'NO DETAIL ELEMENT'
})
