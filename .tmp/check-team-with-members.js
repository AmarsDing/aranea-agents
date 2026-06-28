JSON.stringify((function(){
  const blocks = document.querySelectorAll('.team-stage-block');
  const withMembers = [];
  for (let i = 0; i < blocks.length; i++) {
    const el = blocks[i];
    // Check Vue props for members
    const comp = el.__vueParentComponent;
    if (comp?.props?.activity?.members?.length) {
      withMembers.push({
        idx: i,
        name: el.querySelector('.team-stage-block__name')?.textContent?.slice(0, 40),
        statusBadge: el.querySelector('.team-stage-block__status-badge')?.textContent?.trim().slice(0, 30),
        hasProgressBar: !!el.querySelector('.team-stage-block__bar'),
        progressFillWidth: el.querySelector('.team-stage-block__bar-fill')?.getAttribute('style'),
        hasChevron: !!el.querySelector('.team-stage-block__chevron'),
        hasMembersDiv: !!el.querySelector('.team-stage-block__members'),
        memberCount: el.querySelectorAll('.team-stage-block__member').length,
        domClasses: el.className,
        vueStatus: comp.props.activity.status,
        vueMembers: comp.props.activity.members
      });
    }
  }
  return { totalBlocks: blocks.length, blocksWithMembersProps: withMembers.length, details: withMembers };
})())
