JSON.stringify({
  totalPlans: document.querySelectorAll('.plan-block').length,
  plans: Array.from(document.querySelectorAll('.plan-block')).map((el, i) => ({
    idx: i,
    stepCount: el.querySelectorAll('.plan-block__step, .plan-step, .plan-block__steps > *, .plan-block__step-row').length,
    hasStepsContainer: !!el.querySelector('.plan-block__steps, .plan-block__body, .plan-block__step-list'),
    headerText: el.querySelector('.plan-block__header span:last-child, .plan-block__title')?.textContent?.trim().slice(0, 50),
    bodyVisible: el.querySelector('.plan-block__body, .plan-block__steps')?.children?.length || 0,
    allClasses: Array.from(el.classList).join(',')
  }))
})
