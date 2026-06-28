JSON.stringify((function(){
  const blocks = document.querySelectorAll('.team-stage-block');
  const result = [];
  for (let i = 0; i < Math.min(blocks.length, 5); i++) {
    const el = blocks[i];
    // Try to find Vue component instance
    let vue = el.__vueParentComponent || el.__vnode;
    let props = null;
    try {
      // Quasar/Vue 3 stores component instance on __vueParentComponent
      const comp = el.__vueParentComponent;
      if (comp) {
        const activity = comp.props?.activity;
        if (activity) {
          props = {
            id: activity.id,
            kind: activity.kind,
            status: activity.status,
            teamId: activity.teamId,
            memberCount: activity.members?.length || 0,
            members: activity.members
          };
        }
      }
    } catch(e) { props = 'error: ' + e.message; }
    result.push({
      name: el.querySelector('.team-stage-block__name')?.textContent?.slice(0, 30),
      domClasses: el.className,
      vueProps: props
    });
  }
  return result;
})())
