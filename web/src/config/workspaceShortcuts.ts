// Real product destinations; no sample data or inferred business metrics.
// 域色收敛（05-卡片特色显示）：青=界面/连接、电蓝=数据、AI 紫=智能；状态色不作分类装饰。
export const workspaceShortcuts = [
  { key: 'chat', to: '/chat', icon: 'auto_awesome', tone: 'cyan', featured: true },
  { key: 'tasks', to: '/task-workspace', icon: 'view_quilt', tone: 'blue' },
  { key: 'knowledge', to: '/knowledge', icon: 'library_books', tone: 'blue' },
  { key: 'agents', to: '/agents', icon: 'smart_toy', tone: 'violet' },
  { key: 'workbench', to: '/workbench', icon: 'space_dashboard', tone: 'violet' },
  { key: 'codingTasks', to: '/coding-tasks', icon: 'terminal', tone: 'cyan' },
] as const;
