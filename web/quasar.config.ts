import { configure } from "quasar/wrappers";

export default configure(() => {
  return {
    supportTS: true,
    boot: ["error-handler", "pinia", "theme", "i18n", "runtime", "heartbeat", "close-menu-on-scroll"],
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
        // P0-1 白屏修复（2026-11-09）：禁用 rolldown 自动分包，全部内联为单 bundle。
        // 自动分包会把入口模块与共享模块拆进互相静态引用的 chunk（入口↔router），
        // 跨 chunk 循环初始化导致生产构建顶层求值读到 undefined（整页白屏，
        // 见 docs/development/91-44-deploy-readiness-and-runbook.md §3）。
        // 内网部署单 bundle（gzip 后约 5-6MB）代价可接受；路由仍保持懒加载写法，
        // 未来若恢复分包须先过 scripts/prod-preview-smoke.mjs 门禁。
        viteConf.build = viteConf.build || {};
        viteConf.build.rollupOptions = viteConf.build.rollupOptions || {};
        viteConf.build.rollupOptions.output = {
          ...(viteConf.build.rollupOptions.output || {}),
          codeSplitting: false,
        };
        // dev server 防崩：chokidar 监视 dist 构建产物时一旦文件被杀软/构建锁占用，
        // EBUSY 会作为未处理 'error' 事件直接打崩进程（实测 watch dist/spa/kws/*.data 崩 dev server）。
        // dist 为构建产物，dev 模式无需监听。
        viteConf.server = viteConf.server || {};
        viteConf.server.watch = {
          ...viteConf.server.watch,
          ignored: ["**/dist/**", "**/node_modules/**", "**/.git/**"],
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
