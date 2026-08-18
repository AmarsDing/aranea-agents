import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { quasar, transformAssetUrls } from "@quasar/vite-plugin";

export default defineConfig({
  plugins: [
    vue({
      template: { transformAssetUrls }
    }),
    quasar({
      sassVariables: "src/css/quasar-variables.sass"
    })
  ],
  server: {
    port: 9301,
    proxy: {
      "/v1": { target: "http://127.0.0.1:8810", changeOrigin: true, ws: true },
      "/v2": { target: "http://127.0.0.1:8810", changeOrigin: true },
      "/api": { target: "http://127.0.0.1:8810", changeOrigin: true },
      "/healthz": { target: "http://127.0.0.1:8810", changeOrigin: true }
    }
  },
  preview: {
    port: 9301,
    proxy: {
      "/v1": { target: "http://127.0.0.1:8810", changeOrigin: true, ws: true },
      "/v2": { target: "http://127.0.0.1:8810", changeOrigin: true },
      "/api": { target: "http://127.0.0.1:8810", changeOrigin: true },
      "/healthz": { target: "http://127.0.0.1:8810", changeOrigin: true }
    }
  }
});
