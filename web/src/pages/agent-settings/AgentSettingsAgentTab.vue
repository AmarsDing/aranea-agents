<template>
  <div class="settings-grid">
    <agent-settings-prompt-section
      :system-prompt-mode="form.system_prompt_mode"
      :prompt-modes="promptModes"
      @update:system-prompt-mode="form.system_prompt_mode = $event"
    />

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">Agent 个性</span>
          </div>
          <p class="settings-section__hint">身份、状态、分类与对外描述。</p>
        </div>
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input v-model="form.display_name" dense outlined label="显示名称" />
        <q-input v-model="form.agent_key" dense outlined readonly label="Agent 标识">
          <template #append><q-btn flat round dense icon="content_copy" @click="$emit('copy-key')" /></template>
        </q-input>
        <q-select v-model="form.status" dense outlined emit-value map-options label="状态" :options="statusOptions" />
        <q-toggle v-model="form.is_default" label="默认 Agent" />
        <div class="app-grid-span-full">
          <q-input v-model="form.agent_description" class="app-field-long" outlined autogrow type="textarea" label="专业摘要 / 能力描述" />
          <!-- PGO-3-WEB-03: AI Refine button for agent description -->
          <div class="row justify-end q-mt-xs">
            <AIRefineButton
              scope="agent.description"
              :resource-id="agentId"
              :text="form.agent_description ?? ''"
              flat
              size="sm"
              label="AI 优化描述"
              @apply="(v: string) => { form.agent_description = v }"
            />
          </div>
        </div>
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">模型</span>
          </div>
          <p class="settings-section__hint">选择 Provider 管理中已录入的模型；单价在 Provider 管理中维护。</p>
        </div>
      </div>
      <div class="app-field-md">
        <q-select
          v-model="selectedProviderModelId"
          dense
          outlined
          emit-value
          map-options
          use-input
          fill-input
          hide-selected
          input-debounce="0"
          label="模型"
          hint="仅可选择 Provider 管理中已录入且启用的模型。"
          :options="filteredProviderModelOptions"
          :loading="loadingProviderModels || checkingAgentModel"
          @filter="(val, update) => $emit('filter-provider-models', val, update)"
          @popup-show="$emit('reset-provider-model-filter')"
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
        <div v-if="form.provider && form.model" class="row items-center q-gutter-xs q-mt-xs">
          <q-chip
            v-if="checkingAgentModel"
            dense
            square
            color="blue-grey-2"
            text-color="blue-grey-9"
            icon="hourglass_empty"
            label="校验中…"
          />
          <q-chip
            v-else-if="agentModelCheckOk === true"
            dense
            square
            color="positive"
            text-color="white"
            icon="check_circle"
            label="模型可用"
          />
          <q-chip
            v-else-if="agentModelCheckOk === false"
            dense
            square
            color="negative"
            text-color="white"
            icon="error"
            :label="agentModelCheckMessage || '模型不可用'"
          />
        </div>
      </div>
      <q-banner
        v-if="orphanProviderModel"
        rounded
        class="q-mt-sm settings-warning-banner"
        dense
      >
        当前指向 <code>{{ form.provider }} / {{ form.model }}</code>，但 Provider 目录中无匹配的已启用条目。
        Web、飞书等渠道的消息将无法调用模型。
        <template #action>
          <q-btn flat no-caps color="primary" label="打开模型管理" @click="$emit('open-provider-manager')" />
        </template>
      </q-banner>
      <q-banner
        v-else-if="disabledCatalogMatch"
        rounded
        class="q-mt-sm settings-warning-banner"
        dense
      >
        目录中存在该模型但已禁用，请启用或更换其他模型。
        <template #action>
          <q-btn flat no-caps color="primary" label="打开模型管理" @click="$emit('open-provider-manager')" />
        </template>
      </q-banner>
      <q-banner rounded class="q-mt-md settings-info-banner">
        月度费用上限请在
        <a href="#" class="settings-link" @click.prevent="$emit('open-permissions-tab')">「权限」</a>
        Tab 配置用量配额（Chat Turn 前生效）。
      </q-banner>
    </section>

    <agent-planner-section v-model:form="plannerForm" :model-provider="form.provider" />
    <agent-ralph-loop-section v-model:form="ralphLoopForm" />

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">协作能力</span>
          </div>
          <p class="settings-section__hint">子 Agent 编排与意图预处理。Skill / 工具策略请见「Skill / 工具」Tab。</p>
        </div>
      </div>

      <div class="settings-subsection-grid">
        <div class="settings-subsection">
          <div class="settings-subsection__head row items-center justify-between">
            <div>
              <div class="settings-subsection__title">子 Agent</div>
              <p class="settings-subsection__hint">控制生成限制和归档策略。</p>
            </div>
            <q-toggle v-model="config.subagents.enabled" />
          </div>
          <div v-if="config.subagents.enabled" class="app-form-field-grid app-form-field-grid--2col">
            <q-input v-model.number="config.subagents.max_concurrency" dense outlined type="number" label="最大并发数" />
            <q-input v-model.number="config.subagents.max_generation_depth" dense outlined type="number" label="最大生成深度" />
            <q-input v-model.number="config.subagents.max_children_per_agent" dense outlined type="number" label="每 Agent 最大子数" />
            <q-input v-model.number="config.subagents.archive_after_minutes" dense outlined type="number" label="归档时间 (分钟)" />
            <q-input v-model.number="config.subagents.max_retries" dense outlined type="number" label="最大重试次数" />
            <q-input v-model="config.subagents.model_override" dense outlined label="模型覆盖" placeholder="继承自 Agent" />
          </div>
        </div>

        <div class="settings-subsection">
          <div class="settings-subsection__head row items-center justify-between">
            <div>
              <div class="settings-subsection__title">意图 Pass</div>
              <p class="settings-subsection__hint">
                开启后每轮用户消息进入主模型前先做轻量意图梳理（默认关闭，与后端一致）。全局可用
                <code>ARANEA_INTENT_PASS=1</code> 强制开启。
              </p>
            </div>
            <q-toggle v-model="config.intent_pass.enabled" />
          </div>
        </div>
      </div>

      <q-banner rounded class="q-mt-md settings-info-banner">
        记忆分层、心跳与语义检索请前往
        <a href="#" class="settings-link" @click.prevent="$emit('open-memory-tab')">「记忆」</a>
        Tab 配置。
      </q-banner>
    </section>

    <agent-channel-refs-section :agent-id="agentId" :agent-key="form.agent_key" />
  </div>
</template>

<script setup lang="ts">
import type { Agent } from "../../features/agents/types";
import AgentChannelRefsSection from "./AgentChannelRefsSection.vue";
import AgentSettingsPromptSection from "./AgentSettingsPromptSection.vue";
import AgentPlannerSection from "../../components/agents/AgentPlannerSection.vue";
import AgentRalphLoopSection from "../../components/agents/AgentRalphLoopSection.vue";
import AiRefineButton from "../../components/agents/AIRefineButton.vue";
import type { PlannerFormState } from "../../features/agents/plannerConfig";
import type { RalphLoopFormState } from "../../features/agents/ralphLoopConfig";

const plannerForm = defineModel<PlannerFormState>("plannerForm", { required: true });
const ralphLoopForm = defineModel<RalphLoopFormState>("ralphLoopForm", { required: true });
const selectedProviderModelId = defineModel<string>("selectedProviderModelId", { default: "" });

withDefaults(
  defineProps<{
    form: Agent;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    config: any;
    agentId?: string;
    promptModes?: { value: string; label: string; caption: string; tokens: string }[];
    statusOptions?: { label: string; value: string }[];
    filteredProviderModelOptions?: { label: string; value: string; caption?: string }[];
    loadingProviderModels?: boolean;
    orphanProviderModel?: boolean;
    disabledCatalogMatch?: boolean;
    checkingAgentModel?: boolean;
    agentModelCheckOk?: boolean | null;
    agentModelCheckMessage?: string;
  }>(),
  {
    agentId: "",
    promptModes: () => [],
    statusOptions: () => [],
    filteredProviderModelOptions: () => [],
    loadingProviderModels: false,
    orphanProviderModel: false,
    disabledCatalogMatch: false,
    checkingAgentModel: false,
    agentModelCheckOk: null,
    agentModelCheckMessage: "",
  }
);

defineEmits<{
  "copy-key": [];
  "open-permissions-tab": [];
  "open-memory-tab": [];
  "open-provider-manager": [];
  "filter-provider-models": [val: string, update: (fn: () => void) => void];
  "reset-provider-model-filter": [];
}>();
</script>
