import { configure } from "quasar/wrappers";

export default configure(() => {
  return {
    supportTS: true,
    boot: ["pinia", "theme", "i18n", "runtime", "heartbeat", "close-menu-on-scroll", "liquid-glow"],
    css: ["style.sass"],
    extras: ["material-icons"],
    build: {
      target: {
        browser: ["es2022", "firefox115", "chrome115", "safari14"]
      },
      vueRouterMode: "history",
      rollupOptions: {
        external: []
      },
      extendViteConf(viteConf) {
        viteConf.resolve = viteConf.resolve || {};
        viteConf.resolve.alias = viteConf.resolve.alias || {};
      }
    },
    devServer: {
      // 勿与 configs/config.yaml 中 server.grpc.addr (:9000) 共用端口；前端开发固定 9001。
      // 隔离测试环境可用环境变量覆盖：QUASAR_DEV_PORT / QUASAR_BACKEND_URL。
      port: Number(process.env.QUASAR_DEV_PORT || 9001),
      open: false,
      proxy: (() => {
        const backend = process.env.QUASAR_BACKEND_URL || "http://127.0.0.1:8000";
        return {
          "/v1": {
            target: backend,
            changeOrigin: true,
            ws: true
          },
          "/v2": {
            target: backend,
            changeOrigin: true
          },
          "/api": {
            target: backend,
            changeOrigin: true
          },
          "/healthz": {
            target: backend,
            changeOrigin: true
          }
        };
      })()
    },
    framework: {
      config: { dark: false },
      plugins: ["Dialog", "Notify"]
    },
    animations: []
  };
});
