import { defineBoot } from '#q-app/wrappers';

/**
 * 全局错误可观测性（2026-10-18 可观测性审计 P1-2）。
 *
 * 此前全仓没有 app.config.errorHandler / error / unhandledrejection 监听：
 * API 拦截器之外的异常（组件 async setup、watch、事件 handler、未处理
 * rejection）只在浏览器控制台留下未捕获痕迹，没有任何记录。
 *
 * 统一 console.warn（前端红线仅允许 warn/info）；不弹 toast —— API 错误已
 * 由 axios 拦截器通知，此处只兜底"拦截器之外"的异常，避免重复打扰。
 * Vue 处理错误后继续运行（errorHandler 不重新抛出）。
 */
export default defineBoot(({ app }) => {
  app.config.errorHandler = (err, _instance, info) => {
    console.warn('[vue-error]', info, err);
  };

  window.addEventListener('error', (event) => {
    // 资源加载错误（img/script 等）没有 error 对象，仅记消息与来源。
    console.warn('[window-error]', event.message, event.filename, event.lineno);
  });

  window.addEventListener('unhandledrejection', (event) => {
    console.warn('[unhandled-rejection]', event.reason);
  });
});
