<template>
  <q-dialog :model-value="open" persistent transition-show="slide-up" transition-hide="slide-down" @update:model-value="$emit('update:open', $event)">
    <q-card class="advanced-dialog app-dialog-card app-dialog-card--md app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="row items-center q-gutter-sm">
          <q-icon name="settings" size="22px" color="primary" />
          <div class="text-h6">高级设置</div>
        </div>
        <q-btn flat round icon="close" v-close-popup />
      </q-card-section>

      <q-scroll-area class="app-glass-dialog__body-scroll">
        <div class="q-pa-md q-gutter-md">
          <q-banner rounded dense class="settings-info-banner">
            Ralph Loop（迭代验证）在
            <strong>Agent</strong>
            标签页的「Ralph Loop」区块配置，与规划模式独立。
          </q-banner>
          <section class="settings-section">
            <div class="section-heading">
              <q-icon name="hub" color="primary" size="20px" />
              <div>
                <div class="text-subtitle2 text-weight-bold">通道绑定</div>
                <div class="text-caption text-grey-7">绑定 Agent 与消息入口通道及默认会话。</div>
              </div>
            </div>
            <div class="app-form-field-grid app-form-field-grid--2col">
              <q-select
                v-model="selectedChannelId"
                dense
                outlined
                emit-value
                map-options
                label="Channel"
                :options="channelOptions"
                :loading="loadingChannels"
                hint="选择已配置的消息通道"
                clearable
                @update:model-value="onChannelChange"
              />
              <q-input
                v-model="chatId"
                dense
                outlined
                label="Chat ID"
                hint="外部平台的会话标识（如 chat_id / thread_id）"
                :disable="!selectedChannelId"
              />
            </div>
          </section>

          <section class="settings-section">
            <div class="section-heading">
              <q-icon name="folder_open" color="primary" size="20px" />
              <div>
                <div class="text-subtitle2 text-weight-bold">工作区</div>
                <div class="text-caption text-grey-7">Agent 运行时的文件系统根路径。</div>
              </div>
            </div>
            <q-input v-model="workspace" class="app-field-long" dense outlined label="工作区路径" hint="如 ~/.aranea/workspace/{agent_key}" />
          </section>

          <section class="settings-section">
            <div class="section-heading">
              <q-icon name="psychology" color="primary" size="20px" />
              <div>
                <div class="text-subtitle2 text-weight-bold">扩展思考（Reasoning）</div>
                <div class="text-caption text-grey-7">控制模型推理深度；仅当所选模型支持 Reasoning 时生效。</div>
              </div>
            </div>
            <div class="app-form-field-grid app-form-field-grid--2col">
              <q-select
                v-model="reasoningMode"
                dense
                outlined
                emit-value
                map-options
                label="策略"
                :options="reasoningModeOptions"
              />
              <q-select
                v-model="reasoningLevel"
                dense
                outlined
                emit-value
                map-options
                label="思考级别"
                :options="reasoningLevelOptions"
                :disable="reasoningMode !== 'custom'"
              />
            </div>
          </section>

          <section class="settings-section">
            <div class="section-heading">
              <q-icon name="compress" color="primary" size="20px" />
              <div>
                <div class="text-subtitle2 text-weight-bold">上下文压缩</div>
                <div class="text-caption text-grey-7">长对话自动压缩与摘要策略。</div>
              </div>
            </div>
            <div class="app-form-field-grid app-form-field-grid--2col">
              <q-toggle v-model="compactionEnabled" color="primary" label="启用上下文压缩" />
              <q-toggle v-model="sessionSummaryEnabled" color="primary" label="会话摘要" />
            </div>
          </section>

          <section class="settings-section">
            <div class="section-heading">
              <q-icon name="content_cut" color="primary" size="20px" />
              <div>
                <div class="text-subtitle2 text-weight-bold">上下文裁剪</div>
                <div class="text-caption text-grey-7">控制上下文窗口溢出时的裁剪行为。</div>
              </div>
            </div>
            <div class="app-form-field-grid">
              <q-select
                v-model="truncateStrategy"
                dense
                outlined
                emit-value
                map-options
                label="裁剪策略"
                :options="truncateStrategyOptions"
              />
              <q-input v-model.number="recentWindowTurns" dense outlined type="number" label="保留最近轮数" />
              <q-input v-model.number="recentWindowTokens" dense outlined type="number" label="保留最近 Token" />
              <q-input v-model.number="summaryKeepTurns" dense outlined type="number" label="摘要保留轮数" />
            </div>
          </section>

          <section class="settings-section">
            <div class="section-heading">
              <q-icon name="security" color="primary" size="20px" />
              <div>
                <div class="text-subtitle2 text-weight-bold">沙箱</div>
                <div class="text-caption text-grey-7">工具执行沙箱配置（预留）。</div>
              </div>
            </div>
            <q-banner rounded class="adv-info-banner">
              沙箱配置（镜像、超时、资源限额等）将在后续版本中支持。
            </q-banner>
          </section>
        </div>
      </q-scroll-area>

      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn flat rounded no-caps label="取消" v-close-popup />
        <q-btn color="primary" rounded unelevated no-caps label="保存" :loading="saving" @click="onSave" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";

type ChannelOption = { label: string; value: string; caption?: string };

const props = defineProps<{
  open: boolean;
  saving: boolean;
  channelOptions: ChannelOption[];
  loadingChannels: boolean;
  channelId: string;
  chatIdInput: string;
  workspaceInput: string;
  reasoningModeInput: string;
  reasoningLevelInput: string;
  compactionEnabledInput: boolean;
  sessionSummaryEnabledInput: boolean;
  truncateStrategyInput: string;
  recentWindowTurnsInput: number;
  recentWindowTokensInput: number;
  summaryKeepTurnsInput: number;
}>();

const emit = defineEmits<{
  "update:open": [value: boolean];
  save: [payload: {
    channel_id: string;
    chat_id: string;
    workspace: string;
    reasoning_mode: string;
    reasoning_level: string;
    context_compaction_enabled: boolean;
    session_summary_enabled: boolean;
    truncate_strategy: string;
    recent_window_turns: number;
    recent_window_tokens: number;
    summary_keep_turns: number;
  }];
}>();

const selectedChannelId = ref(props.channelId);
const chatId = ref(props.chatIdInput);
const workspace = ref(props.workspaceInput);
const reasoningMode = ref(props.reasoningModeInput || "provider_default");
const reasoningLevel = ref(props.reasoningLevelInput || "off");
const compactionEnabled = ref(props.compactionEnabledInput);
const sessionSummaryEnabled = ref(props.sessionSummaryEnabledInput);
const truncateStrategy = ref(props.truncateStrategyInput || "sliding");
const recentWindowTurns = ref(props.recentWindowTurnsInput || 20);
const recentWindowTokens = ref(props.recentWindowTokensInput || 0);
const summaryKeepTurns = ref(props.summaryKeepTurnsInput || 4);

const reasoningModeOptions = [
  { label: "跟随厂商", value: "provider_default" },
  { label: "自定义", value: "custom" }
];

const reasoningLevelOptions = [
  { label: "关闭", value: "off" },
  { label: "低（~4K）", value: "low" },
  { label: "中（~10-16K）", value: "medium" },
  { label: "高（~32K）", value: "high" }
];

const truncateStrategyOptions = [
  { label: "滑动窗口", value: "sliding" },
  { label: "摘要优先", value: "summary_first" },
  { label: "严格截断", value: "hard_truncate" }
];

function onChannelChange() {
  chatId.value = "";
}

function onSave() {
  emit("save", {
    channel_id: selectedChannelId.value,
    chat_id: chatId.value,
    workspace: workspace.value,
    reasoning_mode: reasoningMode.value,
    reasoning_level: reasoningLevel.value,
    context_compaction_enabled: compactionEnabled.value,
    session_summary_enabled: sessionSummaryEnabled.value,
    truncate_strategy: truncateStrategy.value,
    recent_window_turns: recentWindowTurns.value,
    recent_window_tokens: recentWindowTokens.value,
    summary_keep_turns: summaryKeepTurns.value
  });
  emit("update:open", false);
}

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    selectedChannelId.value = props.channelId;
    chatId.value = props.chatIdInput;
    workspace.value = props.workspaceInput;
    reasoningMode.value = props.reasoningModeInput || "provider_default";
    reasoningLevel.value = props.reasoningLevelInput || "off";
    compactionEnabled.value = props.compactionEnabledInput;
    sessionSummaryEnabled.value = props.sessionSummaryEnabledInput;
    truncateStrategy.value = props.truncateStrategyInput || "sliding";
    recentWindowTurns.value = props.recentWindowTurnsInput || 20;
    recentWindowTokens.value = props.recentWindowTokensInput || 0;
    summaryKeepTurns.value = props.summaryKeepTurnsInput || 4;
  }
);
</script>
