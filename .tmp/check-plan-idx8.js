JSON.stringify((function(){
  const plans = document.querySelectorAll('.plan-block');
  if (plans.length < 9) return {error: 'only ' + plans.length + ' plans'};
  const p = plans[8];
  const steps = p.querySelectorAll('.plan-block__step');
  return {
    classes: p.className,
    headerText: p.querySelector('.plan-block__header')?.textContent?.trim(),
    countBadge: p.querySelector('.plan-block__count')?.textContent,
    stepCount: steps.length,
    steps: Array.from(steps).map(s => ({
      num: s.querySelector('.plan-block__step-num')?.textContent,
      name: s.querySelector('.plan-block__step-name')?.textContent?.slice(0, 50),
      status: s.querySelector('.plan-block__step-status')?.textContent?.trim().slice(0, 20),
      team: s.querySelector('.plan-block__step-team')?.textContent
    }))
  };
})())
