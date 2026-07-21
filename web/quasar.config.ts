import { configure } from "quasar/wrappers";

export default configure(() => {
  return {
    supportTS: true,
    boot: ["pinia", "theme", "i18n", "runtime", "heartbeat", "close-menu-on-scroll"],
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
      port: 9001,
      open: false,
      proxy: {
        "/v1": {
          target: "http://127.0.0.1:8000",
          changeOrigin: true,
          ws: true
        },
        "/v2": {
          target: "http://127.0.0.1:8000",
          changeOrigin: true
        },
        "/api": {
          target: "http://127.0.0.1:8000",
          changeOrigin: true
        },
        "/healthz": {
          target: "http://127.0.0.1:8000",
          changeOrigin: true
        }
      }
    },
    framework: {
      config: { dark: false },
      plugins: ["Dialog", "Notify"]
    },
    animations: []
  };
});
