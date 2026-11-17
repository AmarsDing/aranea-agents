import { createRouter, createWebHashHistory } from 'vue-router';
import { Screen } from 'quasar';
import { routes } from './routes';

// The desktop app (Tauri) loads the SPA from an embedded loopback origin;
// hash mode keeps deep links stable across both web and desktop deployments.
const history = createWebHashHistory();

// Quasar CLI 约定：这里必须 default export 路由实例或工厂函数（见 .quasar/dev-spa/app.js）
const router = createRouter({
  history,
  routes,
});

// 守卫/钩子依赖一律动态 import：静态引入会让 router chunk 与入口 chunk 互为循环静态依赖，
// Rolldown 下 router 顶层先于入口执行，跨 chunk 顶层调用读到未初始化 binding 而启动崩溃。
router.beforeEach(async (to) => {
  // Android shell without a configured upstream must land on the setup page
  // first. Outside the Tauri embedded proxy fetchBackendConfig() is null and
  // this guard is a no-op.
  const { fetchBackendConfig, MOBILE_SETUP_PATH } = await import('../services/backendConfig');
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
    const { chatBreakpointTarget } = await import('../features/chat/taskWorkspaceNavigation');
    const breakpointTarget = chatBreakpointTarget(to.path, to.query, Screen.lt.sm);
    if (breakpointTarget) return breakpointTarget;
  }

  const { useAuthStore } = await import('../stores/auth');
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

  // 路由级无权限拦截（展示层）：菜单白名单不含目标入口时落到 /403。
  // 后端仍按接口硬判；菜单接口失败时 visiblePaths 为 null，判定放行。
  const { useNavStore } = await import('../stores/nav');
  const nav = useNavStore();
  if (!nav.loaded) {
    try {
      await nav.ensureMenus();
    } catch {
      // 菜单拉取失败 → 保留 null，按下方的降级规则放行
    }
  }
  const { evaluateRouteAccess } = await import('../config/routeAccess');
  const decision = evaluateRouteAccess({
    path: to.path,
    permission: to.meta.permission as string | undefined,
    visiblePaths: nav.visiblePaths,
    permissions: nav.permissions,
  });
  if (!decision.allowed) {
    return {
      path: '/403',
      query: {
        from: to.fullPath,
        ...(decision.reason === 'permission' ? { need: decision.requiredPermission } : {}),
      },
    };
  }
  return true;
});

/**
 * 统一页面身份：标签页标题 + 路由切换后把焦点交给主内容区。
 * 之前全站没有任何 document.title 赋值，多标签页无法区分；焦点停留在侧栏项上
 * 会让键盘用户每次跳转都要重新 Tab 一遍。
 */
router.afterEach(async (to) => {
  const { i18n } = await import('../i18n');
  const appTitle = i18n.global.t('common.appTitle');
  const titleKey = to.meta.titleKey as string | undefined;
  const pageTitle = titleKey ? i18n.global.t(titleKey) : '';
  document.title = pageTitle && pageTitle !== titleKey ? `${pageTitle} · ${appTitle}` : appTitle;

  requestAnimationFrame(() => {
    document.getElementById('app-main-content')?.focus({ preventScroll: true });
  });
});

export default router;
