# 前端参考对齐（adk-web-main）

已对齐的参考点（来自 `design-agent/adk-web-main`）：

- 运行时后端地址注入机制：
  - `clean-config` + `inject-backend` + `serve --backend=...`
  - 文件路径：`frontend/public/assets/config/runtime-config.json`
- 启动时读取 runtime config，再初始化前端应用：
  - `frontend/src/config/runtime.ts`
  - `frontend/src/main.ts`
- API 客户端基地址按运行时配置生成：
  - `frontend/src/api/client.ts`

说明：
- `adk-web-main` 是 Angular 实现；`arenea` 前端保持 Quasar + Vue 技术栈，仅复用可迁移设计思路与契约方式。
