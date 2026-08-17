# 17-frontend 前端测试结果（2026-08-17）

> 形态：dist/spa 构建产物经 `npx serve -l 9301` 静态伺服 + Playwright MCP 真机浏览器（Chromium）。
> 结果：**11 PASS / 1 FAIL**（FE-11 为测试工具缺陷，非被测系统问题）。

## 结果汇总

| 分组 | 用例 | 结果 |
|------|------|------|
| 静态可达性 | FE-01~FE-08 | 8/8 PASS |
| 浏览器渲染 | FE-09/10/12 | PASS |
| 浏览器渲染 | FE-11 | FAIL（MCP 截图不落盘） |

详见 [evidence/results.md](evidence/results.md)。

## 关键验证点

1. **构建产物完整**：index.html 720B 引用 6.0MB JS bundle + 840KB CSS，全部 200；favicon、runtime-config.json 可达。
2. **SPA fallback 正常**：`npx serve` 对 /overview 等前端路由返回 index.html（FE-06）。
3. **SPA 真机挂载成功**：Chromium 加载后 `#q-app` 渲染出完整 q-layout，自动重定向 `/#/overview`，中文导航菜单（概览/对话/Agent/Team/图谱/记忆/知识库/工具/MCP/钩子/可观测/定时任务/设置等）全部渲染。
4. **路由功能正常**：`/#/chat` 可达且自动引导创建 Spirit 会话（session=6c0ec1f0…，agent=agent___spirit__）——证明前端路由 + 会话引导 + 后端 API 链路串通。
5. **WS 跨源连通**：配置注入后，页面上下文新建 `ws://localhost:8810/v1/ws?session_id=*&probe=1` → onopen（OPEN）。

## 问题与原因分析

### P1（已解决·配置缺失）：静态伺服时前端默认回退同源，WS/API 全失败
- **现象**：注入 runtime-config 前，控制台 5 条 WebSocket error，全部指向 `ws://localhost:9301/v1/ws`（静态服务器没有该端点）。证据 `fe10-console-preinjection.log`。
- **原因**：生产构建 `getBackendOrigin()` 在 runtime-config 为空 `{}` 时回退 `window.location.origin`（[runtime.ts](file:///f:/myproject/aranea-agents/web/src/config/runtime.ts#L51-L58)）。dist 产物脱离 admin/nginx 反代单独静态伺服时，同源即错误源。
- **解决**：按设计机制注入 `dist/spa/assets/config/runtime-config.json` = `{"backendUrl":"http://localhost:8810","wsOrigin":"http://localhost:8810"}`（FE-08），重新加载后 0 error，WS probe OPEN。
- **结论**：**非缺陷**，是 runtime-config 机制的标准用法。但建议：部署文档中明确「静态单发 dist 时必须配置 runtime-config.json」，避免运维踩坑。

### P2（测试工具缺陷）：MCP browser_take_screenshot 静默失败
- **现象**：两次截图（fe11-overview.png / fe12-chat.png）MCP 均返回成功与相对链接，但输出目录 `C:\Users\Administrator\.playwright-mcp\` 无 PNG 落盘（同目录 page-*.yml 快照正常写入）。
- **影响**：仅影响测试取证方式，不影响被测系统。已用 a11y snapshot（`fe12-chat-page.yml`，完整渲染树）+ console log 替代视觉证据。
- **解决方案**：后续排查 MCP Playwright 服务的 output-dir 写权限/路径映射；或改用系统级 screenshot 技能。

## 清理

- runtime-config.json 已恢复为原始 `{}`（备份 `fe08-runtime-config.orig.json`；注入版留存 `fe08-runtime-config.active.json` 供复测）。
- 测试残留：FE-12 自动创建的 Spirit 会话 6c0ec1f0（真实会话，可留作后续聊天页深测入口）。
