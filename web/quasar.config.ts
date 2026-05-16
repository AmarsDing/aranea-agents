import { configure } from "quasar/wrappers";

export default configure(() => {
  return {
    supportTS: true,
    boot: ["pinia", "theme", "i18n", "runtime", "heartbeat"],
    css: ["style.sass"],
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
        "/v1": {
          target: "http://127.0.0.1:8000",
          changeOrigin: true
        },
        "/api": {
          target: "http://127.0.0.1:8000",
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
