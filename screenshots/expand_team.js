// Find team-card headers and click to expand
const headers = Array.from(document.querySelectorAll('.team-card__header'));
const result = [];
for (const h of headers) {
  const card = h.closest('.team-card');
  const name = card.querySelector('.team-card__name')?.textContent || '';
  // Only click cards that look like real team cards (not the task overview)
  if (name && !name.startsWith('##')) {
    result.push({ name: name.slice(0, 60), clicked: true });
    h.click();
  } else {
    result.push({ name: name.slice(0, 60), clicked: false, reason: 'skipped' });
  }
}
JSON.stringify(result)
