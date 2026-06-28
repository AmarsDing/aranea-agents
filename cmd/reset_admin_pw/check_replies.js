// Check all reply blocks and their final/intermediate status
const replyBlocks = document.querySelectorAll('[class*="reply-block"], [class*="ReplyBlock"]');
const result = {
  count: replyBlocks.length,
  blocks: []
};
replyBlocks.forEach((el, i) => {
  const text = el.textContent.trim().substring(0, 100);
  const classes = el.className;
  const hasIntermediate = classes.includes('intermediate') || text.includes('中间回复');
  const hasFinal = classes.includes('final') || text.includes('最终回复');
  result.blocks.push({ idx: i, classes, text, hasIntermediate, hasFinal });
});
JSON.stringify(result, null, 2);
