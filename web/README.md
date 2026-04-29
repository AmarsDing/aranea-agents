# Frontend

## 启动
```bash
npm install
npm run dev
```

默认端口 `9000`。

## 参考 adk-web-main 的运行方式
```bash
npm run serve --backend=http://127.0.0.1:8080
```

说明：
- 会先清空并注入 `public/assets/config/runtime-config.json` 中的 `backendUrl`。
- 前端启动时读取该配置并拼接 `/api/v1` 作为 API 基地址。

## MVP 页面
- `/chat`：三栏对话布局（Agent / Session / Chat）。
- `/agents`：Agent 列表与创建。
- `/agents/:id/settings`：设置（Agent、文件两Tab）。
- `/monitor/logs`：基础日志页。
