// Audit plan blocks and their steps
const plans = document.querySelectorAll('.plan-block');
const result = [];
plans.forEach((p, i) => {
  const steps = p.querySelectorAll('.plan-block__step');
  const stepData = [];
  steps.forEach((s, j) => {
    const name = s.querySelector('.plan-block__step-name')?.textContent?.trim();
    const status = s.querySelector('.plan-block__step-status')?.textContent?.trim();
    stepData.push({ name, status });
  });
  const statusText = p.querySelector('.plan-block__status')?.textContent?.trim();
  const summary = p.querySelector('.plan-block__summary')?.textContent?.trim();
  result.push({ index: i, status: statusText, summary, stepCount: steps.length, steps: stepData });
});
JSON.stringify(result, null, 2);
