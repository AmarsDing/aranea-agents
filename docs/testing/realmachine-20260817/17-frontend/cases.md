# 17-frontend 前端测试用例

> 形态：dist/spa 构建产物经 `npx serve -l 9301` 静态伺服；浏览器侧用 Playwright MCP 真机渲染。
> 后端：aranea-admin http://localhost:8810（/healthz /v1/*）。

## 静态可达性（HTTP 层）

| ID | 用例 | 预期 |
|----|------|------|
| FE-01 | GET / 返回 index.html | 200，含 `<title>Aranea Agent Orchestrator</title>` |
| FE-02 | 主 JS bundle 可达 | /assets/index-*.js 200，>1MB |
| FE-03 | 主 CSS 可达 | /assets/index-*.css 200 |
| FE-04 | runtime-config.json 可达且为合法 JSON | 200 + 可解析 |
| FE-05 | favicon.svg 可达 | 200 + image/svg |
| FE-06 | SPA 路由 fallback（/overview） | 200 返回 index.html（或记录实际行为） |
| FE-07 | 后端 /healthz 可达（前端依赖） | 200 |
| FE-08 | runtime-config 注入后端地址后生效 | 配置 backendUrl=8810 后文件可读取 |

## 浏览器渲染（Playwright MCP 真机）

| ID | 用例 | 预期 |
|----|------|------|
| FE-09 | 页面加载渲染 | title 正确，#q-app 有子节点（SPA mount） |
| FE-10 | 控制台错误巡检 | 记录 error 级消息并分析 |
| FE-11 | 概览页截图存档 | 截图保存至 evidence |
| FE-12 | 登录页/路由可达 | /login 或默认重定向页面渲染出表单或主界面 |
