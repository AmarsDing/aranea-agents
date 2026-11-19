import { configure } from "quasar/wrappers";

export default configure(() => {
  return {
    supportTS: true,
    boot: ["error-handler", "pinia", "theme", "a11y", "i18n", "runtime", "heartbeat", "close-menu-on-scroll"],
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
        // dev server 防崩：chokidar 监视 dist 构建产物时一旦文件被杀软/构建锁占用，
        // EBUSY 会作为未处理 'error' 事件直接打崩进程（实测 watch dist/spa/kws/*.data 崩 dev server）。
        // dist 为构建产物，dev 模式无需监听。
        viteConf.server = viteConf.server || {};
        viteConf.server.watch = {
          ...viteConf.server.watch,
          ignored: ["**/dist/**", "**/node_modules/**", "**/.git/**"],
        };

        // Rolldown(Vite 8) 循环 chunk 启动崩溃修复：把 node_modules vendor 抽到独立 chunk。
        // 否则 vue-i18n 的 createI18n 及其内部 helper 会与入口 app 代码同处 index chunk，
        // 被动态 import 拆出的 i18n chunk 顶层反向调用入口尚未初始化的 binding（ll is not a function）而崩溃。
        // vendor 不含 app 代码，i18n/router 等 chunk 只单向依赖 vendor，入口↔vendor 的 eager 环被切断。
        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- rolldownOptions.output.codeSplitting 不在解析到的 Vite 类型里（实际构建走 app-vite 嵌套的 Vite 8/Rolldown）
        const build = (viteConf.build ??= {}) as Record<string, any>;
        const rolldownOptions = (build.rolldownOptions ??= {});
        const output = (rolldownOptions.output ??= {});
        const existing = typeof output.codeSplitting === 'object' && output.codeSplitting ? output.codeSplitting : {};
        output.codeSplitting = {
          ...existing,
          groups: [{ name: 'vendor', test: /node_modules[\\/]/, priority: 10 }, ...(existing.groups ?? [])],
        };
      }
    },
    devServer: {
      // 勿与 configs/config.yaml 中 server.grpc.addr (:9900) 共用端口；前端开发固定 9301（TwinMonitor Web 占用 9001）。
      // 隔离测试环境可用环境变量覆盖：QUASAR_DEV_PORT / QUASAR_BACKEND_URL。
      port: Number(process.env.QUASAR_DEV_PORT || 9301),
      open: false,
      proxy: (() => {
        const backend = process.env.QUASAR_BACKEND_URL || "http://127.0.0.1:8800";
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
