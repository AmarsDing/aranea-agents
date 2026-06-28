JSON.stringify({
  url: location.href,
  planBlock: document.querySelectorAll('.plan-block').length,
  planBlockAny: document.querySelectorAll('[class*="plan-block"]').length,
  teamStageBlock: document.querySelectorAll('.team-stage-block').length,
  teamStageBlockAny: document.querySelectorAll('[class*="team-stage"]').length,
  graphStageBlock: document.querySelectorAll('.graph-stage-block').length,
  thinkingBlock: document.querySelectorAll('.thinking-block, [class*="thinking"]').length,
  replyBlock: document.querySelectorAll('.reply-block, [class*="reply"]').length,
  activityStream: document.querySelectorAll('.activity-stream, [class*="activity-stream"]').length,
  bodyLen: document.body.innerText.length,
  firstHeading: document.querySelector('h1,h2,h3')?.textContent?.slice(0, 80)
})
