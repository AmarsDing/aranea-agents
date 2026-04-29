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
          target: "http://127.0.0.1:8080",
          changeOrigin: true
        },
        "/api": {
          target: "http://127.0.0.1:8080",
          changeOrigin: true
        },
        // ADK 风格 `POST /run_sse`；若 ADK 跑在其它端口，在 runtime-cfg 中设置 adkStreamOrigin
        "/run_sse": {
          target: "http://127.0.0.1:8080",
          changeOrigin: true
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
