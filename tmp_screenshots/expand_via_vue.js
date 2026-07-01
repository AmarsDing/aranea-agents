(function() {
  const card = document.querySelector('.team-card');
  if (!card) return 'no team-card';
  // Try to find Vue component instance
  const keys = Object.keys(card);
  const vueKey = keys.find(k => k.startsWith('__vue'));
  if (!vueKey) return 'no vue instance on card, keys=' + keys.join(',');
  const vm = card[vueKey];
  if (vm && vm.expanded !== undefined) {
    vm.expanded = !vm.expanded;
    if (vm.expanded && vm.emit) {
      const sessionIds = (vm.activity?.members ?? []).map(m => m.session_id).filter(Boolean);
      if (sessionIds.length) vm.emit('expand', sessionIds);
    }
    return 'toggled expanded to ' + vm.expanded;
  }
  return 'vm has no expanded, props=' + Object.keys(vm).slice(0, 20).join(',');
})();
