import { createRouter, createWebHashHistory } from 'vue-router';
import { Screen } from 'quasar';
import { useAuthStore } from '../stores/auth';
import { fetchBackendConfig, MOBILE_SETUP_PATH } from '../services/backendConfig';
import { routes } from './routes';
import { chatBreakpointTarget } from '../features/chat/taskWorkspaceNavigation';

// The desktop app (Tauri) loads the SPA from an embedded loopback origin;
// hash mode keeps deep links stable across both web and desktop deployments.
const history = createWebHashHistory();

// Quasar CLI 约定：这里必须 default export 路由实例或工厂函数（见 .quasar/dev-spa/app.js）
const router = createRouter({
  history,
  routes,
});

router.beforeEach(async (to) => {
  // Android shell without a configured upstream must land on the setup page
  // first. Outside the Tauri embedded proxy fetchBackendConfig() is null and
  // this guard is a no-op.
  const backendCfg = await fetchBackendConfig();
  if (backendCfg?.requiresSetup && to.path !== MOBILE_SETUP_PATH) {
    return { path: MOBILE_SETUP_PATH };
  }

  const isPublic = to.matched.some((r) => r.meta.public === true);
  if (isPublic) return true;
  const requiresAuth = to.matched.some((r) => r.meta.requiresAuth === true);
  if (!requiresAuth) return true;

  // Breakpoint guard (<600px): narrow screens use the mobile layout, wide
  // screens use the desktop one. Evaluated per navigation (no live switching
  // on resize, per design §3.1). The public setup page is exempt, as is the
  // force-reset page (must stay reachable on every form factor).
  if (to.path !== '/force-reset-password') {
    const breakpointTarget = chatBreakpointTarget(to.path, to.query, Screen.lt.sm);
    if (breakpointTarget) return breakpointTarget;
  }

  const auth = useAuthStore();
  await auth.ensureSession();
  if (!auth.user) {
    return { path: '/login', query: { redirect: to.fullPath } };
  }
  // P1 补漏·强制改密门:置位账号钉在改密页(后端同步只放行豁免组 rpc);
  // 未置位账号访问改密页则送回总览。
  if (auth.user.forceReset && to.path !== '/force-reset-password') {
    return { path: '/force-reset-password' };
  }
  if (!auth.user.forceReset && to.path === '/force-reset-password') {
    return { path: '/' };
  }
  return true;
});

export default router;
