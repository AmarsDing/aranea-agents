# Team Flow 画布模块 — 实现设计文档

> 对应需求：`41 team.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Team Flow 可视化画布：复刻 ADK Web 的多智能体编排 UI，用 Quasar (Vue 3) 重建 Builder 模式 + 画布编辑器。

---

## 二、Proto 层

复用 Team 和 Agent 的 Proto，无需新增。

---

## 三、Biz 层

复用 TeamUsecase，无需新增。

---

## 四、Data 层

复用已有 Data 层，无需新增。

---

## 五、运行时层

### 5.1 YAML 生成

```go
// internal/team/yaml_builder.go
func BuildTeamYAML(t biz.Team, members []biz.TeamMember) (map[string]string, error)
```

生成多文件 YAML：
- `agent.yaml` — 主 Agent 配置
- `sub_agents/` — 子 Agent 配置
- `tools.yaml` — 工具配置

---

## 六、Service 层

复用 TeamService，无需新增。

---

## 七、Wire 注入

无需新增。

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/features/teams/
├── api.ts
├── types.ts
├── yamlUtils.ts               ← YAML 生成/解析
└── components/
    ├── TeamFlowPage.vue        ← 画布主页面
    ├── TeamFlowCanvas.vue      ← 画布组件
    ├── TeamFlowNode.vue        ← Agent 节点
    ├── TeamFlowEdge.vue        ← 连线
    ├── TeamFlowGroupNode.vue   ← 分组节点（Sequential/Parallel）
    ├── TeamFlowToolbar.vue     ← 工具栏
    ├── TeamFlowSidebar.vue     ← 侧边栏（Agent 配置）
    └── TeamFlowYamlPreview.vue ← YAML 预览
```

### 8.2 组件设计

**TeamFlowCanvas.vue**：

| 功能 | 实现 | 说明 |
|------|------|------|
| 节点渲染 | `@vue-flow/core` | Agent 节点 |
| 连线 | `@vue-flow/core` | Transfer/AgentTool 关系 |
| 分组 | `@vue-flow/core` Group | Sequential/Parallel 分组 |
| 拖拽 | `@vue-flow/core` DnD | 从侧边栏拖入 Agent |
| 缩放 | `@vue-flow/core` | 画布缩放/平移 |

**TeamFlowNode.vue**：

| 区域 | 组件 | 说明 |
|------|------|------|
| 头像 | `QAvatar` | Agent icon |
| 名称 | `QLabel` | display_name |
| 类型标签 | `QBadge` | coordinator/member |
| 工具横幅 | `QChip` | AgentTool 标识 |

**TeamFlowSidebar.vue**：

| 区域 | 组件 | 说明 |
|------|------|------|
| Agent 列表 | `QList` | 可拖拽 Agent |
| 配置表单 | `QForm` | 选中节点配置 |
| YAML 预览 | `QBtn` | 切换 YAML 视图 |

### 8.3 画布交互

```
1. 从侧边栏拖入 Agent → 创建节点
2. 从节点端口拖线 → 创建边（TransferTool/AgentTool）
3. 双击节点 → 打开配置侧边栏
4. 右键 → 删除/复制/分组
5. 工具栏 → 保存/运行/YAML 预览
```

### 8.4 API

复用 Team API，无需新增。
