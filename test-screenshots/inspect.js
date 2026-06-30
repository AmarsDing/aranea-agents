JSON.stringify({
  // Precise component root counts
  teamCardRoots: document.querySelectorAll('.team-card:not([class*="__"])').length,
  agentCardRoots: document.querySelectorAll('.agent-card:not([class*="__"])').length,
  // Check by exact class
  teamCardExact: document.querySelectorAll('div.team-card').length,
  agentCardExact: document.querySelectorAll('div.agent-card').length,
  // Check team-card__detail (expanded content area)
  teamCardDetails: document.querySelectorAll('.team-card__detail').length,
  agentCardDetails: document.querySelectorAll('.agent-card__detail').length,
  // Check team-card__member (member list items inside TeamCard)
  teamCardMembers: document.querySelectorAll('.team-card__member').length,
  // Get all activity items
  eventStreamItems: document.querySelectorAll('.event-stream').length,
  // Get text content of team card members
  teamMemberTexts: Array.from(document.querySelectorAll('.team-card__member')).map(el => el.textContent?.trim().slice(0, 100)),
  // Check for any session-related elements
  sessionElements: document.querySelectorAll('[class*="session"]').length,
  // Get the team card detail HTML to see what's inside
  teamCardDetailHTML: (document.querySelector('.team-card__detail') || {}).innerHTML?.slice(0, 1500) || 'NO TEAM CARD DETAIL FOUND',
  // Check for agent-card elements anywhere
  agentCardHTML: (document.querySelector('.agent-card') || {}).outerHTML?.slice(0, 500) || 'NO AGENT CARD FOUND'
})
