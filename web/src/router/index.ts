import { createRouter, createWebHashHistory } from 'vue-router';
import { useAuthStore } from '../stores/auth';
import { routes } from './routes';

// The desktop app (Tauri) loads the SPA from an embedded loopback origin;
// hash mode keeps deep links stable across both web and desktop deployments.
const history = createWebHashHistory();

// Quasar CLI 约定：这里必须 default export 路由实例或工厂函数（见 .quasar/dev-spa/app.js）
const router = createRouter({
  history,
  routes,
});

router.beforeEach(async (to) => {
  const isPublic = to.matched.some((r) => r.meta.public === true);
  if (isPublic) return true;
  const requiresAuth = to.matched.some((r) => r.meta.requiresAuth === true);
  if (!requiresAuth) return true;
  const auth = useAuthStore();
  await auth.ensureSession();
  if (!auth.user) {
    return { path: '/login', query: { redirect: to.fullPath } };
  }
  return true;
});

export default router;
