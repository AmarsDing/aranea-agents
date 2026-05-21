<template>
  <div class="settings-grid">
    <agent-settings-prompt-section
      :system-prompt-mode="form.system_prompt_mode"
      :prompt-modes="promptModes"
      @update:system-prompt-mode="form.system_prompt_mode = $event"
    />

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-title">
          <span class="section-title__text">Agent 个性</span>
        </div>
        <div class="text-caption text-grey-7">身份、状态、分类与对外描述。</div>
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input v-model="form.display_name" dense outlined label="显示名称" />
        <q-input v-model="form.agent_key" dense outlined readonly label="Agent 标识">
          <template #append><q-btn flat round dense icon="content_copy" @click="$emit('copy-key')" /></template>
        </q-input>
        <q-select v-model="form.status" dense outlined emit-value map-options label="状态" :options="statusOptions" />
        <q-toggle v-model="form.is_default" color="primary" label="默认 Agent" />
        <q-input v-model="form.agent_description" class="app-field-long" outlined autogrow type="textarea" label="专业摘要 / 能力描述" />
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-title">
          <span class="section-title__text">模型</span>
        </div>
        <div class="text-caption text-grey-7">选择数据库已录入的模型；单价在 Provider 管理中维护并同步至 model_pricing_rules。</div>
      </div>
      <div class="app-field-md">
        <q-select
          :model-value="selectedProviderModelID"
          dense
          outlined
          emit-value
          map-options
          use-input
          input-debounce="0"
          label="模型"
          hint="仅可选择 Provider 管理中已录入且启用的模型。"
          :options="filteredProviderModelOptions"
          :loading="loadingProviderModels"
          :disable="loadingProviderModels"
          @filter="(val, update) => $emit('filter-provider-models', val, update)"
          @update:model-value="$emit('select-provider-model', $event)"
        >
          <template #option="scope">
            <q-item v-bind="scope.itemProps">
              <q-item-section>
                <q-item-label>{{ scope.opt.label }}</q-item-label>
                <q-item-label caption>{{ scope.opt.caption }}</q-item-label>
              </q-item-section>
            </q-item>
          </template>
        </q-select>
      </div>
      <q-banner rounded class="q-mt-md settings-info-banner">
        月度费用上限请在
        <a href="#" class="text-primary" @click.prevent="$emit('open-permissions-tab')">「权限」</a>
        Tab 的「用量配额」中配置（写入 usage_quotas，Chat Turn 前生效）。Agent 表上的 budget_monthly_cents 字段已弃用展示。
      </q-banner>
    </section>

    <agent-planner-section v-model:form="plannerForm" :model-provider="form.provider" />

    <agent-ralph-loop-section v-model:form="ralphLoopForm" />

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-title">
          <span class="section-title__text">能力</span>
        </div>
        <div class="text-caption text-grey-7">子 Agent 与工具策略。冲突工具会在保存前提示。</div>
      </div>
      <div class="row q-col-gutter-md">
        <div class="col-12 col-lg-6">
          <q-card flat bordered class="capability-card">
            <q-card-section class="row items-center justify-between">
              <div>
                <div class="text-subtitle2">子 Agent</div>
                <div class="text-caption text-grey-7">控制生成限制和归档策略。</div>
              </div>
              <q-toggle v-model="config.subagents.enabled" color="primary" />
            </q-card-section>
            <q-separator />
            <q-card-section v-if="config.subagents.enabled" class="app-form-field-grid">
              <q-input v-model.number="config.subagents.max_concurrency" dense outlined type="number" label="最大并发数" />
              <q-input v-model.number="config.subagents.max_generation_depth" dense outlined type="number" label="最大生成深度" />
              <q-input v-model.number="config.subagents.max_children_per_agent" dense outlined type="number" label="每 Agent 最大子数" />
              <q-input v-model.number="config.subagents.archive_after_minutes" dense outlined type="number" label="归档时间 (分钟)" />
              <q-input v-model.number="config.subagents.max_retries" dense outlined type="number" label="最大重试次数" />
              <q-input v-model="config.subagents.model_override" dense outlined label="模型覆盖" placeholder="继承自 Agent" />
            </q-card-section>
          </q-card>
        </div>
        <div class="col-12 col-lg-6">
          <q-card flat bordered class="capability-card">
            <q-card-section class="row items-center justify-between">
              <div>
                <div class="text-subtitle2">工具策略</div>
                <div class="text-caption text-grey-7">控制可调用工具、黑名单与并行白名单。</div>
              </div>
              <q-toggle v-model="config.tools.enabled" color="primary" />
            </q-card-section>
            <q-separator />
            <q-card-section v-if="config.tools.enabled" class="q-gutter-sm">
              <q-select
                v-model="config.tools.profile"
                dense
                outlined
                emit-value
                map-options
                label="工具配置文件"
                hint="按意图选择 Agent 的工具能力面：chat_only 仅对话；read_only 只读 + 时间；coding 代码 + 网页；research 网页 + 检索；full 全开放（高权限）。"
                :options="toolProfileOptions"
              />
              <q-input v-model="config.tools.tool_call_prefix" dense outlined label="工具调用前缀" hint="如 proxy_，解析前会从工具名中剥离。" />
              <q-select
                v-model="config.tools.allow"
                dense
                outlined
                multiple
                use-chips
                emit-value
                map-options
                label="允许"
                :options="toolSelectOptions"
                :loading="loadingCatalogTools"
                hint="选项来自 Tools 目录中的平台工具；亦可保留已保存的自定义 key。"
              />
              <q-select
                v-model="config.tools.deny"
                dense
                outlined
                multiple
                use-chips
                emit-value
                map-options
                label="拒绝"
                :options="toolSelectOptions"
                :loading="loadingCatalogTools"
              />
              <q-select
                v-model="config.tools.concurrent_allow"
                dense
                outlined
                multiple
                use-chips
                emit-value
                map-options
                label="同时允许"
                :options="toolSelectOptions"
                :loading="loadingCatalogTools"
              />
              <q-banner v-if="toolConflicts.length" rounded class="settings-warning-banner">
                以下工具同时出现在允许与拒绝中，运行时按拒绝优先：{{ toolConflicts.join(", ") }}
              </q-banner>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-12">
          <agent-tools-section :agent-id="agentId" />
        </div>
        <div class="col-12 col-lg-6">
          <q-card flat bordered class="capability-card">
            <q-card-section class="row items-center justify-between">
              <div>
                <div class="text-subtitle2">工具重试</div>
                <div class="text-caption text-grey-7">工具调用失败时自动重试，指数退避 + 随机抖动。</div>
              </div>
              <q-toggle v-model="config.tools.retry.enabled" color="primary" />
            </q-card-section>
            <q-separator />
            <q-card-section v-if="config.tools.retry.enabled" class="app-form-field-grid app-form-field-grid--2col">
              <q-input v-model.number="config.tools.retry.max_attempts" dense outlined type="number" label="最大重试次数" hint="含首次调用" />
              <q-input v-model.number="config.tools.retry.initial_interval_ms" dense outlined type="number" label="初始间隔 (ms)" />
              <q-input v-model.number="config.tools.retry.backoff_factor" dense outlined type="number" step="0.1" label="退避因子" />
              <q-input v-model.number="config.tools.retry.max_interval_ms" dense outlined type="number" label="最大间隔 (ms)" />
              <q-toggle v-model="config.tools.retry.jitter" color="primary" label="随机抖动" />
            </q-card-section>
          </q-card>
        </div>
        <div class="col-12 col-lg-6">
          <q-card flat bordered class="capability-card">
            <q-card-section class="row items-center justify-between">
              <div>
                <div class="text-subtitle2">并行与流式</div>
                <div class="text-caption text-grey-7">并行执行多个工具调用；流式工具支持需工具实现 StreamableCall 接口。</div>
              </div>
            </q-card-section>
            <q-separator />
            <q-card-section class="app-form-field-grid app-form-field-grid--2col">
              <q-toggle v-model="config.tools.parallel_enabled" color="primary" label="并行工具调用" hint="模型发出多个工具调用时并行执行" />
              <q-toggle v-model="config.tools.streaming_enabled" color="primary" label="流式工具" hint="启用支持 StreamableCall 的工具流式输出" />
            </q-card-section>
          </q-card>
        </div>
        <div class="col-12">
          <q-card flat bordered class="capability-card">
            <q-card-section class="row items-center justify-between">
              <div>
                <div class="text-subtitle2">意图 Pass</div>
                <div class="text-caption text-grey-7">
                  每轮用户消息进入主模型前先做一次轻量意图梳理（多一次 LLM 调用）。部署侧可用环境变量
                  <code>ARANEA_INTENT_PASS=0</code>
                  等对全部 Agent 做全局覆写。
                </div>
              </div>
              <q-toggle v-model="config.intent_pass.enabled" color="primary" />
            </q-card-section>
          </q-card>
        </div>
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-title">
          <span class="section-title__text">记忆与心跳</span>
        </div>
        <div class="text-caption text-grey-7">语义检索、Dreaming 与 HEARTBEAT.MD 注入。</div>
      </div>
      <div class="app-form-field-grid">
        <q-toggle v-model="config.memory.enabled" color="primary" label="记忆启用" />
        <q-input v-model.number="config.memory.max_chunk_length" dense outlined type="number" label="最大块长度" />
        <q-input v-model.number="config.memory.max_results" dense outlined type="number" label="最大结果数" />
        <q-input v-model.number="config.memory.min_score" dense outlined type="number" step="0.01" label="最低分数" />
        <q-toggle v-model="config.heartbeat.enabled" color="negative" label="心跳启用" />
        <q-input v-model.number="config.heartbeat.interval_minutes" dense outlined type="number" suffix="min" label="间隔" />
        <q-input v-model="heartbeatFile.body" class="app-field-long" dense outlined autogrow type="textarea" label="检查清单 (HEARTBEAT.MD)" />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import type { Agent } from "../../features/agents/types";
import type { AgentFile } from "../../components/agents/agentUi";
import AgentSettingsPromptSection from "./AgentSettingsPromptSection.vue";
import AgentToolsSection from "../../components/agents/AgentToolsSection.vue";
import AgentPlannerSection from "../../components/agents/AgentPlannerSection.vue";
import AgentRalphLoopSection from "../../components/agents/AgentRalphLoopSection.vue";
import type { PlannerFormState } from "../../features/agents/plannerConfig";
import type { RalphLoopFormState } from "../../features/agents/ralphLoopConfig";

const plannerForm = defineModel<PlannerFormState>("plannerForm", { required: true });
const ralphLoopForm = defineModel<RalphLoopFormState>("ralphLoopForm", { required: true });

withDefaults(
  defineProps<{
    form: Agent;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    config: any;
    agentId?: string;
    promptModes?: { value: string; label: string; caption: string; tokens: string }[];
    statusOptions?: { label: string; value: string }[];
    selectedProviderModelID?: string;
    filteredProviderModelOptions?: { label: string; value: string; caption?: string }[];
    loadingProviderModels?: boolean;
    toolProfileOptions?: { label: string; value: string }[];
    toolSelectOptions?: { label: string; value: string }[];
    loadingCatalogTools?: boolean;
    toolConflicts?: string[];
    heartbeatFile?: AgentFile;
  }>(),
  {
    agentId: "",
    promptModes: () => [],
    statusOptions: () => [],
    selectedProviderModelID: "",
    filteredProviderModelOptions: () => [],
    loadingProviderModels: false,
    toolProfileOptions: () => [],
    toolSelectOptions: () => [],
    loadingCatalogTools: false,
    toolConflicts: () => [],
    heartbeatFile: () => ({ name: "HEARTBEAT.MD", caption: "", body: "" })
  }
);

defineEmits<{
  "copy-key": [];
  "open-permissions-tab": [];
  "filter-provider-models": [val: string, update: (fn: () => void) => void];
  "select-provider-model": [value: string];
}>();
</script>

<style scoped>
/* 组件特有样式；通用 .settings-section / .capability-card 由 agent-settings-page.scss 控制 */
.settings-info-banner,
.settings-warning-banner {
  background: var(--glass-elevated);
  color: var(--color-text-secondary);
}
</style>
