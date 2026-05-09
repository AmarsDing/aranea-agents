import { configure } from "quasar/wrappers";

export default configure(() => {
  return {
    supportTS: true,
    boot: ["pinia", "theme", "i18n", "runtime"],
    css: ["style.sass"],
    // 必须开启，否则 QIcon 会回退为显示 name 的文本，侧栏会叠字
    extras: ["material-icons"],
    build: {
      target: {
        browser: ["es2022", "firefox115", "chrome115", "safari14"]
      },
      vueRouterMode: "history",
      distDir: "dist"
    },
    devServer: {
      port: 9000,
      open: false,
      proxy: {
        // Kratos Admin：`/v1/admins/...`（与 `web/src/services` 中 `adminApi` 对齐）
        "/v1": {
          target: "http://127.0.0.1:8000",
          changeOrigin: true
        },
        "/api": {
          target: "http://127.0.0.1:8000",
          changeOrigin: true
        },
        // ADK 风格 `POST /run_sse`；若 ADK 跑在其它端口，在 runtime-cfg 中设置 adkStreamOrigin
        "/run_sse": {
          target: "http://127.0.0.1:8000",
          changeOrigin: true
        },
        // tx7do SSE（configs server.sse.addr，默认 :8001）。
        // Vite 代理须使用 `rewrite`；`pathRewrite`（webpack）会被忽略，未剥离 `/sse` 时后端只有 `/monitor/...` 路由 → 404 → 「连接异常」。
        "/sse": {
          target: "http://127.0.0.1:8001",
          changeOrigin: true,
          timeout: 0,
          rewrite: (path: string) => path.replace(/^\/sse/, "") || "/"
        }
      }
    },
    framework: {
      config: { dark: false },
      plugins: ["Notify"]
    },
    animations: []
  };
});
