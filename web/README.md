# Frontend

## 启动
```bash
npm install
npm run dev
```

开发服务器默认端口 **9001**（避免与后端 gRPC `:9000` 冲突）。API 经 Vite 代理到 `http://127.0.0.1:8000`。

## 参考 adk-web-main 的运行方式
```bash
npm run serve --backend=http://127.0.0.1:8000
```

说明：
- 会先清空并注入 `public/assets/config/runtime-config.json` 中的 `backendUrl`。
- 前端 API 统一走 **`/v1/...`**（`kratosApi`）；`backendUrl` 应指向 admin HTTP（默认 **`http://127.0.0.1:8000`**，不是 :8080）。

## MVP 页面
- `/chat`：三栏对话布局（Agent / Session / Chat）。
- `/agents`：Agent 列表与创建。
- `/agents/:id/settings`：设置（Agent、文件两Tab）。
- `/monitor/logs`：基础日志页。
