# 4 团队管理（ADK Web → Quasar 重建）

本文档约定：**「Team flow」= ADK 内置 Web UI 中的多智能体编排能力**（`sub_agents`、Agent Tool、画布上的工作流分组），对应仓库 [adk-web](https://github.com/google/adk-web) 在本地的副本：`f:\project\aranea\adk-web-main`（Angular + Material + `ngx-vflow`）。目标是用 **Quasar（Vue 3）** 复刻同一套 **与 `adk api_server` 的契约**，而不是重写 ADK 运行时。

---

## 0. 先读懂 ADK Web 里「团队」实际长什么样

| 概念 | 在 adk-web-main 中的位置 | 说明 |
|------|---------------------------|------|
| 总壳 | `src/app/app.component.html` → `<app-chat>` | 单页入口 |
| 会话 / 调试侧栏 | `src/app/components/chat/chat.component.html` | `mat-drawer`：应用选择、Session、Events、Trace、Artifacts、Eval 等 |
| **Builder / 多 Agent 团队** | 同文件 `@if (isBuilderMode())` | 左侧 `app-builder-tabs` + 主区 `app-canvas` |
| 画布图 | `src/app/components/canvas/canvas.component.html` | `ngx-vflow`：节点/边、分组（Sequential/Parallel 等）、**Add sub-agent**、Agent Tool 横幅 |
| 表单与 YAML 逻辑 | `src/app/components/builder-tabs/builder-tabs.component.ts` | Agent 配置、工具、回调；配合 `YamlUtils` |
| YAML 生成 | `src/utils/yaml-utils.ts` | `sub_agents`、`AgentTool` 等到多文件 YAML |
| 后端 API 封装 | `src/app/core/services/agent.service.ts` | `run_sse`、`list-apps`、`/builder/*`、`/dev/build_graph/*` |
| 后端地址 | `src/utils/url-util.ts`、`set-backend.js` → `src/assets/config/runtime-config.json` | `window.runtimeConfig.backendUrl` |

**结论**：Quasar 侧要复刻的是：**Builder 模式 UI + 与 `adk api_server` 相同的 HTTP/SSE 调用**；智能体执行语义仍由 ADK Python 服务提供。

---

## 1. 环境与依赖（与官方 ADK Web 一致）

1. 安装 **Node.js / npm**。
2. 安装 **Python `google-adk`**，确保本机可运行：
   - `adk api_server --allow_origins=<你的 Quasar 开发源> --host=0.0.0.0`
3. 可选：保留 `adk-web-main` 用于对照调试（`npm run serve --backend=http://127.0.0.1:8000` 与 Angular 行为对比）。

---

## 2. 新建 Quasar 工程

```bash
npm init quasar@latest
```

建议选择：

- **Vue 3 + TypeScript + Vite**
- 启用 **ESLint**（按需）
- 包管理器与团队规范一致即可

进入项目目录后：

```bash
npm install
```

---

## 3. 配置 ADK 后端地址（对齐 `runtimeConfig.backendUrl`）

Angular 版通过 `set-backend.js` 写入 `runtime-config.json`，运行时读 `window['runtimeConfig'].backendUrl`。

**Quasar 推荐两种等价做法（二选一）：**

**方案 A — 构建期注入（简单）**

- 在 `quasar.config` 的 `build.env` 或 `define` 中增加例如 `VITE_ADK_API_BASE=http://127.0.0.1:8000`。
- 封装 `getApiBaseUrl()`：优先 `import.meta.env.VITE_ADK_API_BASE`，便于本地/生产切换。

**方案 B — 运行时配置（更接近 adk-web）**

- 将 `public/runtime-config.json` 置于静态目录，应用 `fetch('/runtime-config.json')` 后写入 `pinia` 或全局单例，再发起 API 请求。
- 部署时只替换该 JSON，无需重新打包。

**CORS**：`adk api_server` 的 `--allow_origins` 必须包含 Quasar dev 的 origin（例如 `http://localhost:9000`，以你实际端口为准）。

---

## 4. 实现与 `agent.service.ts` 等价的 API 层

在 Quasar 中建 `src/services/adkApi.ts`（或 composable），**路径与请求体保持与现有 ADK Web 一致**（参考 `f:\project\aranea\adk-web-main\src\app\core\services\agent.service.ts`）：

| 方法 | HTTP | 用途 |
|------|------|------|
| `runSse` | `POST {base}/run_sse`，`Accept: text/event-stream`，body 为 `AgentRunRequest` | 流式对话 / 执行 |
| `listApps` | `GET {base}/list-apps?relative_path=./` | 应用列表 |
| `agentBuild` | `POST {base}/builder/save` | 保存 Builder 生成的应用 |
| `agentBuildTmp` | `POST {base}/builder/save?tmp=true` | 临时保存 |
| `getAgentBuilder` | `GET {base}/builder/app/{appName}?ts=...` | 拉取 Builder 数据 |
| `getAgentBuilderTmp` / `getSubAgentBuilder` | 带 `tmp=true`、`file_path=` | 子 Agent 文件编辑 |
| `agentChangeCancel` | `POST {base}/builder/app/{appName}/cancel` | 取消未保存变更 |
| `getAppInfo` | `GET {base}/dev/build_graph/{appName}` | 构建图信息（若画布需与后端图同步） |

**SSE 解析**：沿用 Angular 版逻辑——按行过滤 `data:`，逐条 `JSON.parse`；不完整 chunk 拼到缓冲区（见 `runSse` 实现）。

其余资源（Artifacts、Events 图、Eval 等）在 `src/app/core/services/` 下可按需逐个端口；**团队画布核心**先保证 `builder/*` + `run_sse` + `list-apps`。

---

## 5. 数据模型与 YAML 生成（从 Angular 平移）

1. 将 `AgentNode`、`ToolNode`、`CallbackNode`、`YamlConfig` 等类型从  
   `adk-web-main\src\app\core\models\AgentBuilder`  
   迁到 Quasar 的 `src/types/agent-builder.ts`（路径可调，类型保持一致）。
2. 将 `yaml-utils.ts` 迁为纯 TS 模块（无 Angular 依赖），供「保存」「导出」调用。
3. **不要改变**生成 YAML 的字段约定（`sub_agents`、`config_path`、`AgentTool` 等），否则 ADK 加载会失败。

---

## 6. Quasar UI：映射 ADK Web 的「团队 / Builder」布局

参考 `chat.component.html` 的结构，用 Quasar 组件重组：

| ADK Web | Quasar 建议 |
|---------|-------------|
| `mat-drawer` 侧栏 | `QLayout` + `QDrawer`（或 `QSplitter` 左右分栏） |
| Builder 左侧 `app-builder-tabs` | 独立 `BuilderTabs.vue`：`QTabs` / `QExpansionItem` + 表单 |
| 主区 `app-canvas` | 新 `TeamCanvas.vue`：图编辑组件（见下） |
| 顶部 Accept / Close / Assistant | `QBtn` + `QToolbar` |
| Agent Tool 横幅 | `QBanner` 或顶栏 `QChip` + 返回主画布 |

**交互要点（与 canvas 对齐）**：

- 分组节点表示 `agent_class`（Sequential / Parallel / Loop 等）。
- 空分组内 **Add sub-agent**：弹出菜单选择 `LlmAgent` 等类型（见 `canvas.component.html`）。
- **Agent Tool** 子画布：`currentAgentTool` 状态时显示返回主画布。

---

## 7. 画布技术选型（替代 `ngx-vflow`）

Angular 使用 `ngx-vflow`。在 Vue 生态可选：

- [**Vue Flow**](https://vueflow.dev/)（`@vue-flow/core`）：节点/边/分组与 ADK 画布较接近，社区活跃。
- 或 **Cytoscape.js** / **JointJS**（偏重或需授权时注意许可证）。

**实施步骤**：

1. `npm install @vue-flow/core`（及所需样式）。
2. 定义 node/edge 的 `data` 结构与 `adk-web-main` 中 `vflowNodes`、`edges` 生成逻辑对齐（可从 `canvas.component.ts` 抄数据塑形思路）。
3. 点击分组 / 节点时，同步到 `BuilderTabs` 当前选中的 `AgentNode`（Pinia 存「当前选中 agent」）。

---

## 8. 主流程（与 ADK Web 一致的操作顺序）

1. 启动 `adk api_server`（含正确 `--allow_origins`）。
2. Quasar `quasar dev`，确认能 `GET list-apps`。
3. 侧栏选择 app → 进入 **Builder 模式**（等价「进入团队编排」）。
4. 在画布增删子 Agent、配置工具与回调；左侧表单与画布双向绑定。
5. **Accept**：调用 `POST /builder/save`（或 `tmp=true` 试存），与 Angular `saveAgentBuilder()` 行为一致。
6. 退出 Builder → 回到会话视图；`POST /run_sse` 用同一 `appName` / `userId` / `sessionId` 验证运行。

---

## 9. 测试与验收清单（Team 相关）

- [ ] `list-apps` 与 Angular 同一后端结果一致。
- [ ] Builder 保存后，磁盘上 YAML 目录结构与 `yaml-utils` 生成一致。
- [ ] 画布：多层级 `sub_agents`、Agent Tool 钻取与返回。
- [ ] `run_sse` 能收到与官方 UI 相同结构的 event JSON（便于后续 Events 面板移植）。
- [ ] 跨域：仅 Quasar origin 白名单，无浏览器 CORS 报错。

---

## 10. 参考文件速查（adk-web-main）

```
f:\project\aranea\adk-web-main\src\app\components\chat\chat.component.html
f:\project\aranea\adk-web-main\src\app\components\canvas\canvas.component.ts
f:\project\aranea\adk-web-main\src\app\components\builder-tabs\builder-tabs.component.ts
f:\project\aranea\adk-web-main\src\app\core\services\agent.service.ts
f:\project\aranea\adk-web-main\src\utils\yaml-utils.ts
f:\project\aranea\adk-web-main\set-backend.js
f:\project\aranea\adk-web-main\README.md
```

---

## 11. 文档与许可

- ADK 文档：<https://google.github.io/adk-docs/>
- `adk-web-main` 为 Apache 2.0；若直接复制源码片段，需保留许可证头注释或按项目合规要求处理。
