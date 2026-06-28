JSON.stringify({
  url: location.href,
  planBlocks: document.querySelectorAll('.plan-block').length,
  teamStageBlocks: document.querySelectorAll('.team-stage-block').length,
  planSteps: document.querySelectorAll('.plan-block__step, .plan-step, .plan-block__steps > *').length,
  teamMembers: document.querySelectorAll('.team-stage-block__member, .team-member, .team-stage-block__members > *').length,
  bodyLen: document.body.innerText.length,
  hasLogin: !!document.querySelector('input[type=password]'),
  firstHeading: (document.querySelector('h1,h2,h3')||{}).innerText
})
