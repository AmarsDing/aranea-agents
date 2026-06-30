(() => {
  // Click the first agent-card header to expand it and see execution process
  const agentHeaders = document.querySelectorAll('.agent-card__header');
  if (agentHeaders.length === 0) return 'No agent-card headers found';
  agentHeaders[0].click();
  return `Clicked agent-card header 0, total: ${agentHeaders.length}`;
})()
