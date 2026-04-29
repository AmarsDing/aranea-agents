import { createRouter, createWebHistory } from "vue-router";
import { routes } from "./routes";

// Quasar CLI 约定：这里必须 default export 路由实例或工厂函数（见 .quasar/dev-spa/app.js）
const router = createRouter({
  history: createWebHistory(),
  routes
});

export default router;
