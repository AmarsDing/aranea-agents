import { defineConfig } from "vitest/config";
import vue from "@vitejs/plugin-vue";
import { fileURLToPath } from "url";
import { resolve } from "path";

export default defineConfig({
  plugins: [vue()],
  test: {
    environment: "happy-dom",
    globals: true,
    include: ["src/**/__tests__/**/*.spec.ts", "src/**/*.spec.ts"],
    coverage: {
      provider: "v8",
      reporter: ["text", "lcov"],
      include: ["src/stores/**", "src/features/**"]
    }
  },
  resolve: {
    alias: {
      src: resolve(fileURLToPath(new URL(".", import.meta.url)), "src")
    }
  }
});
