// Audit the activity tree structure - find all plan blocks and their parent hierarchy
(() => {
const plans = document.querySelectorAll('.plan-block');
const result = [];
plans.forEach((p, i) => {
  // Walk up the DOM to find parent activity containers
  const parents = [];
  let el = p.parentElement;
  while (el && !el.classList.contains('chat-message-panel')) {
    if (el.classList.contains('activity-stream') || el.classList.contains('activity-item') || el.dataset?.activityId) {
      parents.push({
        tag: el.tagName,
        class: el.className?.substring?.(0, 80),
        activityId: el.dataset?.activityId,
      });
    }
    el = el.parentElement;
    if (parents.length > 5) break;
  }
  // Get the plan's first step name for identification
  const firstStep = p.querySelector('.plan-block__step-name')?.textContent?.trim();
  result.push({ index: i, firstStep, parents });
});
return JSON.stringify(result, null, 2);
})();
