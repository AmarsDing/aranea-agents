# CLI 终端控制台 — 实现设计文档

> 对应需求：`25 cli.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Aranea CLI：终端命令行工具，支持直接命令模式和交互式对话模式。内置系统管家 Agent，通过自然语言驱动跨模块操作。

---

## 二、架构设计

### 2.1 二进制结构

```
cmd/cli/
└── main.go          ← CLI 入口
```

### 2.2 依赖

- `github.com/spf13/cobra` — 命令框架
- `github.com/charmbracelet/bubbletea` — TUI 框架
- 后端 REST API — 通过 HTTP 调用

---

## 三、命令体系

### 3.1 直接命令模式

```
aranea <resource> <action> [flags]

Resources: skill, agent, team, tool, plugin, mcp, cron, channel, session, monitor, system
Actions:   list, get, create, update, delete, install, uninstall, toggle, test, run
```

### 3.2 对话模式

```
$ aranea          ← 进入 REPL
aranea> 帮我安装 figma-code-connect skill
aranea> 列出所有 Agent
aranea> 退出
```

---

## 四、Biz 层

### 4.1 系统管家 Agent

```go
type SystemAdminAgent struct {
    client *http.Client
    baseURL string
}

func (a *SystemAdminAgent) HandleMessage(ctx, msg string) (string, error)
```

### 4.2 内置工具集

```go
var SystemAdminTools = []string{
    "skill_install_from_url",
    "skill_list",
    "agent_list",
    "agent_create",
    "team_list",
    "tool_list",
    "mcp_server_list",
    "cron_job_list",
    "channel_list",
    "system_status",
}
```

---

## 五、Data 层

### 5.1 API Client

```go
// internal/cli/api_client.go
type APIClient struct {
    baseURL    string
    httpClient *http.Client
    token      string
}

func (c *APIClient) ListAgents(ctx) ([]Agent, error)
func (c *APIClient) CreateAgent(ctx, req) (Agent, error)
// ... 所有资源 CRUD
```

### 5.2 配置管理

```go
// internal/cli/config.go
type CLIConfig struct {
    BaseURL  string `yaml:"base_url"`
    Token    string `yaml:"token"`
    Output   string `yaml:"output"`  // json/table/yaml
}
```

---

## 六、Web 前端设计

CLI 为独立终端工具，无 Web 前端。

---

## 七、Wire 注入

独立二进制，不通过 Wire 注入。
