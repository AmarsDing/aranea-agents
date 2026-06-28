JSON.stringify({
  totalTeams: document.querySelectorAll('.team-stage-block').length,
  teamsWithMembers: document.querySelectorAll('.team-stage-block__members').length,
  first3Teams: Array.from(document.querySelectorAll('.team-stage-block')).slice(0, 3).map(el => ({
    name: el.querySelector('.team-stage-block__name')?.textContent?.slice(0, 40),
    status: el.querySelector('.team-stage-block__status-badge')?.textContent?.trim().slice(0, 30),
    hasMembersDiv: !!el.querySelector('.team-stage-block__members'),
    memberCount: el.querySelectorAll('.team-stage-block__member').length,
    classes: el.className
  })),
  last3Teams: Array.from(document.querySelectorAll('.team-stage-block')).slice(-3).map(el => ({
    name: el.querySelector('.team-stage-block__name')?.textContent?.slice(0, 40),
    hasMembersDiv: !!el.querySelector('.team-stage-block__members'),
    memberCount: el.querySelectorAll('.team-stage-block__member').length
  }))
})
