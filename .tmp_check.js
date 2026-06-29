(function(){
  // Find the main chat message area
  var main = document.querySelector('.chat-messages-main') || document.querySelector('.chat-mid-card');
  if (!main) return JSON.stringify({error: 'no main chat area found'});
  // Get the visible text content (truncated)
  var text = main.innerText || main.textContent || '';
  // Get structural info
  var teamCards = main.querySelectorAll('.team-card');
  var planBlocks = main.querySelectorAll('.plan-block');
  var graphBlocks = main.querySelectorAll('.graph-stage-block');
  var agentCards = main.querySelectorAll('.agent-card');
  var thinkingBlocks = main.querySelectorAll('.thinking-block, [class*="thinking"]');
  var actionBlocks = main.querySelectorAll('.action-block, [class*="action"]');
  var replyBlocks = main.querySelectorAll('.reply-block, [class*="reply"]');
  var userBubbles = main.querySelectorAll('.user-bubble, [class*="user-message"]');
  return JSON.stringify({
    textLen: text.length,
    textPreview: text.substring(0, 2000),
    counts: {
      teamCards: teamCards.length,
      planBlocks: planBlocks.length,
      graphBlocks: graphBlocks.length,
      agentCards: agentCards.length,
      thinkingBlocks: thinkingBlocks.length,
      actionBlocks: actionBlocks.length,
      replyBlocks: replyBlocks.length,
      userBubbles: userBubbles.length
    }
  });
})()
