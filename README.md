# Kratos Admin Template（aranea-agents）

后台管理服务与原生 Agent / 聊天 / 团队编排；**业务运行时以 `internal` + Kratos 为主**，Agent 框架参考 **`pkg/adk-go`**（Google ADK Go），旧版能力仍在 **`pkg/backend`**（仅兼容、迁移对照时参阅）。

---

## 项目结构（目录树与职责）

以下为仓库内主要目录；`pkg/adk-go`、`pkg/backend` 体量较大，此处只列顶层子模块含义，不逐文件展开。

```
aranea-agents/
├── .cursor/                  # Cursor IDE 规则与 Agent 提示
├── .tools/                   # 本地工具/缓存（若有）
├── _scratch_bind/            # 本地草稿（勿依赖）
├── api/kratos/               # Protobuf API 定义（按领域分子目录 → 生成 Go/前端客户端）
│   ├── admin/ agent/ agent_category/ avatar/ channel/ chat/ cron/ hook/
│   ├── llm_provider_model/ mcp_server/ memory/ monitor/ plugin/ session/
│   ├── skill/ team/ tool/ usage/
│   └── ...
├── cmd/                      # 可执行程序入口
│   ├── admin/                # 主服务：HTTP/gRPC、业务 API、聊天、Agent
│   └── data/                 # 数据相关 CLI（如有）
├── configs/                  # 运行时配置（如 config.yaml、smoke.yaml）
├── docs/                     # 设计与迁移文档
│   ├── API/ assets/ migration/ model/ UI/ vue-design/
│   └── ...
├── internal/                 # 应用核心（优先维护）
│   ├── agent/                # Catalog Agent：OpenAI 兼容轮、提示、选项；ADK（llmagent/Runner 辅助见 adk_*.go、adksvc/）
│   ├── channel/              # 渠道领域占位（ADK 扩展：`channel/adk/`，待补充）
│   ├── biz/                  # 领域用例与仓储接口（业务规则）
│   ├── conf/                 # 配置加载与绑定
│   ├── cronrunner/           # 定时任务执行
│   ├── data/                 # Ent 数据访问、仓储实现、种子数据、会话记忆 SQL
│   ├── legacychat/           # 遗留聊天 REST 代理/兼容
│   ├── llminspect/           # LLM 调试/探针辅助
│   ├── mcpprobe/             # MCP 探测或实验代码
│   ├── pkg/                  # 内部小型库（如 skillstorage）
│   ├── provider/             # 厂商 LLM 注册表与 ADK model.LLM 解析（如 adk_llm.go）
│   ├── server/               # Kratos 服务器：HTTP/gRPC 路由、中间件注册
│   ├── service/              # 用例门面：将 RPC/HTTP 接到 biz + Agent 运行时
│   ├── skill/                # Skill：`importer/`（ZIP 导入）、`watch/`（磁盘监听同步）
│   ├── team/                 # 团队定义与工作流（顺序/并行/循环等）
│   └── tools/                # 内置 Tool；registry 装配；catalog.Options 高级组合
├── output/                  # 本地生成物输出目录（通常应忽略，不提交）
├── pkg/                      # 可复用子工程/依赖树
│   ├── adk-go/               # Google ADK Go（Agent/Runner/LLM/tool；框架真相源）
│   ├── auth/                 # 认证相关公共包
│   ├── backend/              # 旧版单体/兼容代码与文档（新功能勿照抄分层）
│   ├── docs/                 # 与 backend 配套的长文档
│   └── validate/             # 校验相关
├── scripts/                  # 构建、生成、运维脚本
├── third_party/              # 第三方 proto 等
└── web/                      # 管理端前端（Vue/Quasar）
    ├── public/
    ├── src/
        ├── boot/             # 启动与挂载
        ├── components/       # 通用 UI 组件
        ├── config/           # 前端配置与预设
        ├── css/
        ├── features/         # 按领域封装的 API 与类型（agents、tools、teams…）
        ├── i18n/
        ├── layouts/
        ├── pages/            # 页面级路由视图
        ├── router/           # 路由表
        ├── services/         # Kratos 生成的 TS 客户端与封装入口
        └── stores/           # Pinia 状态
```

### 顶层目录说明

| 目录 | 作用 |
|------|------|
| `api/kratos` | 接口契约：`.proto` 定义 RPC/HTTP，经 `make api` 生成 Go 与 `web/src/services/kratos`。 |
| `cmd` | 程序入口；`cmd/admin` 为对外主进程。 |
| `configs` | YAML 配置（端口、数据库、超时、SSE 等）。 |
| `docs` | 功能/迁移/模型/UI 说明文档。 |
| `internal` | **主业务代码**：server → service → biz → data；ADK 相关拆在 `agent`（含 `adksvc`）、`provider`、`tools`、`team` 等，无单独适配包。 |
| `pkg/adk-go` | ADK 框架实现（Runner、Session、LLM、Tool、工作流 Agent）。 |
| `pkg/backend` | 历史实现与大量设计文档；新功能分层请以 `internal` + ADK 为准。 |
| `web` | 管理后台 SPA：Agent/会话/Tools/团队/渠道等。 |
| `third_party` | 外部 proto 依赖。 |
| `scripts` | 自动化脚本。 |

### `internal/service` 与运行时边界（简要）

- **HTTP/gRPC 入口**在 `internal/server`，实现通常在 `internal/service`。
- **原生聊天/Agent 轮次**使用 `pkg/adk-go` 的 Runner 时，由 `internal/service` 组合 `internal/agent`（构建 llmagent、内存/插件）、`internal/agent/adksvc`（会话快照）、`internal/tools`（工具有效列表）等；边界说明见 `docs/AGENT_RUNTIME_BOUNDARY.md`。

---

## Best Practice
Google AIP(https://google.aip.dev/general):
1. Resource-oriented design
2. Filtering
3. Pagination
4. Field masks
5. Field behavior

## Generate API files
```shell
# Download and update dependencies
make init
# Generate API files (include: pb.go, http, grpc, validate, swagger, index.ts) by proto file
make api
```

## Run Web Application
```shell
# Enter web directory, install dependencies and start development server
cd web
npm install
npm run dev
```

The generated clients work with any Promise-based HTTP client that returns JSON.  
Services are defined and re-exported from this file: `web/src/services/index.ts`.  
```typescript
import { createAdminServiceClient } from "@/services/kratos/admin/v1/index";

type Request = {
  path: string;
  method: string;
  body: string | null;
};

function requestHandler({ path, method, body }: Request) { ... }

export function createAdminService() {
  return createAdminServiceClient(requestHandler);
}
```

Example using the generated client:
```typescript
import { createAdminService } from "@/services/index";

const adminService = createAdminService();

const handleLogin = async (username: string, password: string) => {
  try {
    const response = await adminService.Login({
      username: username,
      password: password,
    });
    console.log("Login successful:", response);
  } catch (error) {
    console.error("Login failed:", error);
  }
};
```
