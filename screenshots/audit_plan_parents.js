// Check the activity tree data from the Vue component
(() => {
  // Find the ActivityStream component and inspect its props
  const stream = document.querySelector('.activity-stream');
  if (!stream) return JSON.stringify({ error: 'no .activity-stream found' });

  // Try to access Vue's internal instance
  const vueEl = stream.__vue_app__ || stream.__vueParentComponent;
  if (!vueEl) return JSON.stringify({ error: 'no vue instance found' });

  // Walk up to find the root app and access Pinia/state
  let app = window.__VUE_APP__;
  if (!app) {
    // Try to find via the element's component
    const comp = stream.__vueParentComponent;
    if (comp) {
      const props = comp.props;
      if (props?.activityTree) {
        const plans = props.activityTree.filter((n) => n.kind === 'plan');
        return JSON.stringify(plans.map((p) => ({
          id: p.id,
          kind: p.kind,
          parentActivityId: p.parentActivityId,
          status: p.status,
          firstStepLabel: p.meta?.steps?.[0]?.label,
          turnId: p.turnId,
        })), null, 2);
      }
    }
  }
  return JSON.stringify({ error: 'could not access activityTree' });
})();
