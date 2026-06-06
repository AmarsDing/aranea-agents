# Agent 顶栏与系统提示词 — 实现设计文档

> 对应需求：`8 agent-title.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 详情页顶栏（身份摘要、标签、操作按钮）和系统提示词预览对话框。顶栏组合展示 Agent 核心属性，系统提示词对话框按模式预览运行态渲染后的完整系统提示词。

本模块主要是前端组合展示，后端已有 `GetAgent` + `GetAgentPromptPreview` RPC 支撑，无需新增后端接口。

---

## 二、Proto 层

### 2.1 现有 Proto（`api/kratos/agent/v1/agent.proto`）

```protobuf
message Agent {
  string id = 1;
  string agent_key = 2;
  string display_name = 3;
  string provider = 4;
  string model = 5;
  string status = 6;
  bool is_default = 7;
  bool is_favorite = 8;
  string icon = 9;
  string agent_description = 10;
  string category_position_id = 11;
  string system_prompt_mode = 12;
  int32 context_window = 13;
  int32 budget_monthly_cents = 14;
  string config_json = 15;
  string created_at = 16;
  string updated_at = 17;
  string deleted_at = 18;
  AgentRuntimeSettings settings = 19;
  repeated AgentPromptFile files = 20;
}

message GetAgentPromptPreviewRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string mode = 2; // "complete" | "task" | "minimized" | "none"
}

message GetAgentPromptPreviewResponse {
  string preview = 1;
}

service AgentService {
  rpc GetAgent(GetAgentRequest) returns (Agent) {
    option (google.api.http) = {get: "/v1/agents/{id}"};
  }
  rpc UpdateAgent(UpdateAgentRequest) returns (Agent) {
    option (google.api.http) = {patch: "/v1/agents/{id}" body: "agent"};
  }
  rpc DeleteAgent(DeleteAgentRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {delete: "/v1/agents/{id}"};
  }
  rpc GetAgentPromptPreview(GetAgentPromptPreviewRequest) returns (GetAgentPromptPreviewResponse) {
    option (google.api.http) = {get: "/v1/agents/{id}/system-prompt/preview"};
  }
}
```

### 2.2 无需新增

顶栏和预览对话框是纯前端组合，调用已有 API。`GetAgentPromptPreview` 已支持按 mode 预览。

---

## 三、Biz 层

### 3.1 Prompt 预览（已有实现）

```go
// internal/agent/prompt.go — 已实现

func BuildSystemPrompt(agent biz.Agent, files []biz.AgentPromptFile, mode string) string {
    filtered := biz.FilesForMode(files, mode)
    var b strings.Builder
    if d := strings.TrimSpace(agent.AgentDescription); d != "" {
        b.WriteString(d)
        b.WriteString("\n\n")
    }
    for _, f := range filtered {
        if body := strings.TrimSpace(f.Body); body != "" {
            b.WriteString(fmt.Sprintf("<internal_config name=%q>\n", f.Name))
            b.WriteString(body)
            b.WriteString("\n</internal_config>\n\n")
        }
    }
    return strings.TrimSpace(b.String())
}
```

### 3.2 FilesForMode（已有实现）

```go
// internal/biz/agent_defaults.go — 已实现

func FilesForMode(files []AgentPromptFile, mode string) []AgentPromptFile {
    if mode == "complete" || mode == "" {
        return files
    }
    allowed := map[string]bool{}
    switch mode {
    case "task":
        allowed = map[string]bool{
            "AGENTS_CORE.md": true, "AGENTS_TASK.md": true,
            "IDENTITY.md": true, "CAPABILITIES.md": true,
            "RULE.md": true, "HEARTBEAT.md": true,
        }
    case "minimized":
        allowed = map[string]bool{
            "AGENTS_CORE.md": true, "IDENTITY.md": true, "RULE.md": true,
        }
    case "none":
        return nil
    }
    var out []AgentPromptFile
    for _, f := range files {
        if allowed[f.Name] {
            out = append(out, f)
        }
    }
    return out
}
```

---

## 四、Data 层

无需新增。`GetAgent` 已返回完整 Agent（含 settings + files），`GetAgentPromptPreview` 在 Service 层直接调用 `BuildSystemPrompt` 组装。

---

## 五、Service 层

### 5.1 GetAgentPromptPreview（已有实现）

```go
// internal/service/agent.go — 已实现

func (s *AgentService) GetAgentPromptPreview(ctx context.Context, req *v1.GetAgentPromptPreviewRequest) (*v1.GetAgentPromptPreviewResponse, error) {
    ag, err := s.uc.Get(ctx, req.GetId())
    if err != nil {
        return nil, err
    }
    files, _ := s.uc.ListAgentPromptFiles(ctx, req.GetId())
    mode := req.GetMode()
    if mode == "" {
        mode = ag.SystemPromptMode
    }
    preview := agent.BuildSystemPrompt(ag, files, mode)
    return &v1.GetAgentPromptPreviewResponse{Preview: preview}, nil
}
```

---

## 六、Wire 注入

已有，无需新增。

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/agents/
├── api.ts                              ← 新增 getPromptPreview
├── types.ts                            ← 已有 Agent 类型
└── components/
    ├── AgentHeader.vue                 ← 顶栏组件
    └── PromptPreviewDialog.vue         ← 系统提示词预览对话框
```

### 7.2 TypeScript 类型

```typescript
// web/src/features/agents/types.ts — 已有，无需新增

// system_prompt_mode 取值
export type SystemPromptMode = "complete" | "task" | "minimized" | "none";

// PromptPreviewResponse
export type PromptPreviewResponse = {
  preview: string;
};
```

### 7.3 API 调用

```typescript
// web/src/features/agents/api.ts 新增

export async function getPromptPreview(
  agentId: string,
  mode: string
): Promise<string> {
  const { data } = await http.get(
    `/v1/agents/${agentId}/system-prompt/preview`,
    { params: { mode } }
  );
  return data?.preview ?? "";
}
```

### 7.4 Vue 组件 — AgentHeader.vue

```vue
<template>
  <div class="agent-header q-pa-md">
    <div class="row items-center no-wrap">
      <!-- 返回 -->
      <QBtn flat round dense icon="arrow_back" @click="$router.back()">
        <QTooltip>返回列表</QTooltip>
      </QBtn>

      <!-- 头像 -->
      <QAvatar size="40px" class="q-mx-sm cursor-pointer" @click="onAvatarClick">
        <img v-if="agent.icon" :src="agent.icon" />
        <QIcon v-else name="smart_toy" size="28px" />
      </QAvatar>

      <!-- 名称 + 标签 -->
      <div class="col q-mx-sm">
        <div class="row items-center q-gutter-xs">
          <span class="text-h6">{{ agent.display_name }}</span>
          <QIcon
            :name="agent.is_favorite ? 'star' : 'star_border'"
            :color="agent.is_favorite ? 'amber' : 'grey'"
            class="cursor-pointer"
            @click="onToggleFavorite"
          >
            <QTooltip>{{ agent.is_favorite ? '取消收藏' : '收藏' }}</QTooltip>
          </QIcon>
          <QBadge v-if="agent.status === 'active'" color="positive" rounded>在线</QBadge>
        </div>
        <div class="row items-center q-gutter-xs q-mt-xs">
          <QChip dense size="sm" :color="modeChipColor" text-color="white">
            {{ modeLabel }}
          </QChip>
          <QChip v-if="agent.settings?.evolution_self_evolve" dense size="sm" color="purple" text-color="white">
            进化中
          </QChip>
          <span class="text-caption text-grey">
            {{ agent.agent_key }} · {{ agent.provider }} / {{ agent.model }}
          </span>
        </div>
      </div>

      <!-- 操作按钮 -->
      <div class="row q-gutter-xs">
        <QBtn flat round dense icon="visibility" @click="showPreview = true">
          <QTooltip>系统提示词</QTooltip>
        </QBtn>
        <QBtn flat round dense icon="settings" @click="$emit('open-advanced')">
          <QTooltip>高级设置</QTooltip>
        </QBtn>
        <QBtn flat round dense icon="delete" color="negative" @click="onDelete">
          <QTooltip>删除</QTooltip>
        </QBtn>
      </div>
    </div>

    <!-- 系统提示词预览对话框 -->
    <PromptPreviewDialog
      v-model="showPreview"
      :agent-id="agent.id"
      :default-mode="agent.system_prompt_mode"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from "vue";
import { useQuasar } from "quasar";
import type { Agent } from "../types";
import { updateAgent, deleteAgent } from "../api";
import PromptPreviewDialog from "./PromptPreviewDialog.vue";

const props = defineProps<{
  agent: Agent;
}>();

const emit = defineEmits<{
  (e: "updated", agent: Agent): void;
  (e: "open-advanced"): void;
  (e: "open-avatar-picker"): void;
}>();

const $q = useQuasar();
const showPreview = ref(false);

const modeLabel = computed(() => {
  const map: Record<string, string> = {
    complete: "完整",
    task: "任务",
    minimized: "最小化",
    none: "无",
  };
  return map[props.agent.system_prompt_mode] ?? "完整";
});

const modeChipColor = computed(() => {
  const map: Record<string, string> = {
    complete: "primary",
    task: "teal",
    minimized: "orange",
    none: "grey",
  };
  return map[props.agent.system_prompt_mode] ?? "primary";
});

async function onToggleFavorite() {
  try {
    const updated = await updateAgent(props.agent.id, {
      is_favorite: !props.agent.is_favorite,
    });
    emit("updated", updated);
  } catch { /* ignore */ }
}

function onAvatarClick() {
  emit("open-avatar-picker");
}

async function onDelete() {
  $q.dialog({
    title: "确认删除",
    message: `确定要删除 Agent「${props.agent.display_name}」吗？此操作不可撤销。`,
    cancel: true,
    persistent: true,
  }).onOk(async () => {
    try {
      await deleteAgent(props.agent.id);
      $q.notify({ type: "positive", message: "已删除" });
    } catch {
      $q.notify({ type: "negative", message: "删除失败" });
    }
  });
}
</script>
```

### 7.5 Vue 组件 — PromptPreviewDialog.vue

```vue
<template>
  <QDialog v-model="modelValue" maximized transition-show="slide-up" transition-hide="slide-down">
    <QCard class="bg-dark text-white">
      <QBar class="bg-primary text-white">
        <QIcon name="visibility" />
        <div class="text-subtitle1 q-ml-sm">系统提示词</div>
        <QSpace />
        <QBadge v-if="estimatedTokens > 0" color="info" class="q-mr-sm">
          ~{{ estimatedTokens.toLocaleString() }} tokens
        </QBadge>
        <QBtn flat round dense icon="close" @click="modelValue = false" />
      </QBar>

      <QTabs v-model="activeMode" dense inline-label class="text-grey" active-color="primary" indicator-color="primary">
        <QTab name="complete" label="完整" />
        <QTab name="task" label="任务" />
        <QTab name="minimized" label="最小化" />
        <QTab name="none" label="无" />
      </QTabs>

      <QSeparator />

      <QTabPanels v-model="activeMode" class="bg-dark">
        <QTabPanel v-for="m in ['complete', 'task', 'minimized', 'none']" :key="m" :name="m">
          <div class="row justify-end q-mb-sm">
            <QBtn flat dense icon="content_copy" label="复制" @click="onCopy" />
          </div>
          <QScrollArea style="height: calc(100vh - 180px)">
            <div v-if="loading" class="column items-center q-pa-lg">
              <QSpinner size="32px" color="primary" />
            </div>
            <pre v-else-if="previewText" class="preview-content text-body2" style="white-space: pre-wrap; word-break: break-word;">{{ previewText }}</pre>
            <div v-else class="column items-center q-pa-lg text-grey-6">
              <QIcon name="block" size="48px" class="q-mb-sm" />
              <div>该模式下无系统提示词</div>
            </div>
          </QScrollArea>
        </QTabPanel>
      </QTabPanels>
    </QCard>
  </QDialog>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
import { useQuasar } from "quasar";
import { getPromptPreview } from "../api";

const props = defineProps<{
  agentId: string;
  defaultMode: string;
}>();

const modelValue = defineModel<boolean>({ default: false });
const $q = useQuasar();

const activeMode = ref(props.defaultMode || "complete");
const previewText = ref("");
const estimatedTokens = ref(0);
const loading = ref(false);

const cache: Record<string, string> = {};

watch(modelValue, (open) => {
  if (open) {
    activeMode.value = props.defaultMode || "complete";
    loadPreview(activeMode.value);
  }
});

watch(activeMode, (mode) => {
  loadPreview(mode);
});

async function loadPreview(mode: string) {
  if (cache[mode]) {
    previewText.value = cache[mode];
    estimatedTokens.value = Math.ceil(cache[mode].length / 4);
    return;
  }
  loading.value = true;
  try {
    const text = await getPromptPreview(props.agentId, mode);
    cache[mode] = text;
    previewText.value = text;
    estimatedTokens.value = Math.ceil(text.length / 4);
  } catch {
    previewText.value = "";
    estimatedTokens.value = 0;
  } finally {
    loading.value = false;
  }
}

function onCopy() {
  navigator.clipboard.writeText(previewText.value);
  $q.notify({ type: "positive", message: "已复制到剪贴板" });
}
</script>
```

### 7.6 高级设置模态（概要）

高级设置模态由 `8 agent-title.md` §8 定义，包含供应商-模型级联、通道-Chat 级联、工作区、扩展思考等区块。各区块的具体设计分别对应：

| 区块 | 设计文档 |
|------|----------|
| 供应商与模型级联 | `9 provider.design.md` |
| 通道与 Chat 级联 | `17 channel.design.md` |
| 工作区 | `5 agent-setting.design.md` |
| 扩展思考（Reasoning） | `5 agent-setting.design.md` |
| 压缩/裁剪/沙箱 | `5 agent-setting.design.md` |

高级设置模态本身是一个 `QDialog`，内部使用 `QScrollArea` + 多个 `QCard` 分段，每个区块的表单字段和交互逻辑参考对应设计文档。

---

## 八、验收要点

- [ ] 顶栏完整 / 任务 / 最小化 / 无 标签与 `system_prompt_mode` 一致
- [ ] 「进化中」标签与 `evolution_self_evolve` 策略一致
- [ ] 系统提示词对话框 Token 估算与四子 Tab 预览与后端渲染一致
- [ ] `AGENTS_CORE` / `AGENTS_TASK` 在完整/任务/最小化模式下出现规则与 `6 agent-setting-file.md` 一致
- [ ] 收藏星标切换正确调用 `UpdateAgent` 并更新 UI
- [ ] 删除按钮弹出确认对话框，确认后调用 `DeleteAgent`
- [ ] 高级中供应商 → 模型级联与 `9 provider.design.md` 一致
- [ ] 高级中 Channel → Chat ID 级联与 `17 channel.design.md` 一致
