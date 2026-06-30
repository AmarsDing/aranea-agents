// Access the Vue app's activity data via the component tree
(() => {
  // Find the root Vue app element
  const app = document.querySelector('#q-app') || document.querySelector('#app');
  if (!app || !app.__vue_app__) return JSON.stringify({ error: 'no vue app found' });

  const vueApp = app.__vue_app__;
  const root = vueApp._instance;

  // Walk the component tree to find ChatMessagePanel or ActivityStream
  function findComponent(instance, depth = 0) {
    if (depth > 20) return null;
    if (!instance) return null;

    // Check if this component has activityTree prop
    const props = instance.props;
    if (props && props.activityTree !== undefined) {
      return instance;
    }

    // Check subTree and children
    const subTree = instance.subTree;
    if (subTree) {
      const found = findInVnode(subTree, depth);
      if (found) return found;
    }
    return null;
  }

  function findInVnode(vnode, depth) {
    if (!vnode) return null;
    if (vnode.component) {
      const found = findComponent(vnode.component, depth + 1);
      if (found) return found;
    }
    if (vnode.children && Array.isArray(vnode.children)) {
      for (const child of vnode.children) {
        const found = findInVnode(child, depth);
        if (found) return found;
      }
    }
    if (vnode.dynamicChildren && Array.isArray(vnode.dynamicChildren)) {
      for (const child of vnode.dynamicChildren) {
        const found = findInVnode(child, depth);
        if (found) return found;
      }
    }
    return null;
  }

  const comp = findComponent(root);
  if (!comp) return JSON.stringify({ error: 'no component with activityTree found' });

  const tree = comp.props.activityTree;
  if (!tree) return JSON.stringify({ error: 'activityTree is null/empty' });

  // Find all plans in the tree (root level and nested)
  const allPlans = [];
  function collectPlans(nodes, level = 0) {
    if (!Array.isArray(nodes)) return;
    for (const n of nodes) {
      if (n.kind === 'plan') {
        allPlans.push({
          id: n.id,
          level,
          parentActivityId: n.parentActivityId,
          status: n.status,
          turnId: n.turnId,
          firstStep: n.meta?.steps?.[0]?.label,
          childCount: n.children?.length ?? 0,
        });
      }
      if (n.children?.length) collectPlans(n.children, level + 1);
    }
  }
  collectPlans(tree);

  // Also find all tasks
  const allTasks = [];
  for (const n of tree) {
    if (n.kind === 'task') {
      allTasks.push({
        id: n.id,
        status: n.status,
        childCount: n.children?.length ?? 0,
        childKinds: (n.children || []).map(c => c.kind),
      });
    }
  }

  return JSON.stringify({
    rootCount: tree.length,
    rootKinds: tree.map(n => n.kind),
    plans: allPlans,
    tasks: allTasks,
  }, null, 2);
})();
