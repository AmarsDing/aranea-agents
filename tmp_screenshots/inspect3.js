JSON.stringify({
  planBlock: (() => {
    const b = document.querySelector('.plan-block');
    if (!b) return null;
    return {
      classList: b.className,
      innerHTML_length: b.innerHTML.length,
      innerHTML: b.innerHTML.slice(0, 2000),
      children: Array.from(b.children).map(c => ({ cls: c.className, tag: c.tagName, text: c.textContent.trim().slice(0, 200) }))
    };
  })(),
  // Try alternative selectors for plan items
  planItems: Array.from(document.querySelectorAll('.plan-block__step, .plan-block__steps, .plan-block__item')).map(el => ({
    cls: el.className,
    text: el.textContent.trim().slice(0, 100)
  })),
  // Try alternative selectors for action blocks
  actionBlocksSel: Array.from(document.querySelectorAll('[class*="action"], [class*="Action"], [data-kind="action"]')).map(el => ({
    cls: el.className,
    tag: el.tagName
  })).slice(0, 10),
  // Check what kind of "thinking" elements exist
  thinkingBlocksDetail: Array.from(document.querySelectorAll('[class*="thinking"], [class*="Thinking"]')).slice(0, 3).map(el => ({
    cls: el.className,
    tag: el.tagName,
    collapsed: el.classList.contains('collapsed') || el.classList.contains('thinking-block--collapsed'),
    text: el.textContent.trim().slice(0, 80)
  }))
}, null, 2)
