(() => {
  const result = {
    teamStages: [],
    graphStages: [],
    thinkingBlocks: [],
    actionBlocks: [],
    replyBlocks: [],
    summary: {},
  };

  // 1. TeamStageBlock
  document.querySelectorAll('.team-stage, [class*="team-stage"]').forEach((el, i) => {
    if (i < 5) {
      result.teamStages.push({
        className: el.className.slice(0, 120),
        title: el.querySelector('[class*="title"], [class*="header"]')?.textContent?.trim().slice(0, 80) || '',
        progress: el.querySelector('[class*="progress"]')?.textContent?.trim() || '',
        statusIcon: el.querySelector('[class*="status"]')?.textContent?.trim() || '',
        members: Array.from(el.querySelectorAll('[class*="member"]')).map((m) => ({
          name: m.textContent?.trim().slice(0, 50) || '',
          statusDot: m.querySelector('[class*="dot"], [class*="status"]')?.className || '',
        })),
      });
    }
  });

  // 2. GraphStageBlock
  document.querySelectorAll('.graph-stage, [class*="graph-stage"]').forEach((el, i) => {
    if (i < 5) {
      result.graphStages.push({
        className: el.className.slice(0, 120),
        title: el.querySelector('[class*="title"], [class*="header"]')?.textContent?.trim().slice(0, 80) || '',
        progress: el.querySelector('[class*="progress"]')?.textContent?.trim() || '',
        nodes: Array.from(el.querySelectorAll('[class*="node"]')).map((n) => ({
          label: n.textContent?.trim().slice(0, 50) || '',
        })),
      });
    }
  });

  // 3. ThinkingBlock labels (检查是否有"规划"/"推理"等细分标签)
  document.querySelectorAll('.thinking-block').forEach((el, i) => {
    if (i < 8) {
      result.thinkingBlocks.push({
        labelText: el.querySelector('.thinking-block__label-text')?.textContent?.trim() || '',
        streamingText: el.querySelector('.thinking-block__streaming-text')?.textContent?.trim() || '',
        isStreaming: el.className.includes('thinking-block--streaming'),
      });
    }
  });

  // 4. ActionBlock status (检查是否还是全 failed)
  const actions = document.querySelectorAll('.act-activity');
  const statusCounts = {};
  actions.forEach((el) => {
    const statusSpan = el.querySelector('.act-activity__status');
    const cls = statusSpan?.className || '';
    const match = cls.match(/act-activity__status--(\w+)/);
    const status = match ? match[1] : 'none';
    statusCounts[status] = (statusCounts[status] || 0) + 1;
  });
  result.summary.actionStatusCounts = statusCounts;

  // 5. ReplyBlock labels (检查是否还是全"中间回复")
  document.querySelectorAll('.reply-block').forEach((el, i) => {
    if (i < 5) {
      const label = el.querySelector('.reply-block__label-text')?.textContent?.trim() || '';
      result.replyBlocks.push({
        labelText: label,
        isStreaming: el.className.includes('reply-block--streaming'),
      });
    }
  });

  result.summary = {
    ...result.summary,
    totalTeamStages: result.teamStages.length,
    totalGraphStages: result.graphStages.length,
    totalThinkingBlocks: document.querySelectorAll('.thinking-block').length,
    totalActionBlocks: actions.length,
    totalReplyBlocks: document.querySelectorAll('.reply-block').length,
    uniqueThinkingLabels: [...new Set(result.thinkingBlocks.map((t) => t.labelText))],
    uniqueReplyLabels: [...new Set(result.replyBlocks.map((r) => r.labelText))],
  };

  return JSON.stringify(result, null, 2);
})();
