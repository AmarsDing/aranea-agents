JSON.stringify({
  bodyTextLen: document.body.innerText.length,
  bodyTextHead: document.body.innerText.slice(0, 4000),
  teamCards: Array.from(document.querySelectorAll(".team-card")).map(c => c.innerText.slice(0, 500)),
  agentCards: Array.from(document.querySelectorAll(".agent-card")).map(c => c.innerText.slice(0, 300)),
  actionBlockCount: document.querySelectorAll(".action-block, [class*=action]").length,
  replyBlockCount: document.querySelectorAll(".reply-block, [class*=reply]").length,
  thinkingBlockCount: document.querySelectorAll(".thinking-block, [class*=thinking]").length,
  planBlockCount: document.querySelectorAll(".plan-block, [class*=plan]").length,
  graphBlockCount: document.querySelectorAll(".graph-stage-block, [class*=graph]").length,
  buttons: Array.from(document.querySelectorAll("button")).map(b => b.innerText.trim()).filter(t => t),
}, null, 2)
