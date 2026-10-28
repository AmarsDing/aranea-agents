// Real product destinations; no sample data or inferred business metrics.
export const workspaceShortcuts = [
  { key: 'chat', to: '/chat', icon: 'auto_awesome', tone: 'cyan', featured: true },
  { key: 'tasks', to: '/task-workspace', icon: 'view_quilt', tone: 'violet' },
  { key: 'knowledge', to: '/knowledge', icon: 'library_books', tone: 'green' },
  { key: 'agents', to: '/agents', icon: 'smart_toy', tone: 'amber' },
  { key: 'workbench', to: '/workbench', icon: 'space_dashboard', tone: 'pink' },
] as const;
