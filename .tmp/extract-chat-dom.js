JSON.stringify({
  classes: Array.from(document.querySelectorAll('*')).map(function(e){return e.className}).filter(function(c){return typeof c === 'string' && /activity|thinking|action|reply|tool|stream|turn|bubble/i.test(c)}).slice(0, 100),
  thinkingRegions: document.querySelectorAll('[role=region][aria-label*="思考"]').length,
  toolEntries: document.querySelectorAll('[class*="tool"], [class*="action"]').length,
  replyEntries: document.querySelectorAll('[class*="reply"]').length,
  headings: Array.from(document.querySelectorAll('h1,h2,h3')).map(function(h){return h.tagName + ': ' + h.textContent.slice(0,50)}).slice(-10)
}, null, 2)
