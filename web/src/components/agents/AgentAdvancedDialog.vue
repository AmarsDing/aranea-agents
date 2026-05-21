<template>
  <q-dialog :model-value="open" persistent transition-show="slide-up" transition-hide="slide-down" @update:model-value="$emit('update:open', $event)">
    <q-card class="advanced-dialog">
      <q-card-section class="row items-center justify-between dialog-header">
        <div class="row items-center q-gutter-sm">
          <q-icon name="settings" size="22px" color="primary" />
          <div class="text-h6">高级设置</div>
        </div>
        <q-btn flat round icon="close" v-close-popup />
      </q-card-section>

      <q-scroll-area class="dialog-body">
        <div class="q-pa-md q-gutter-md">
          <q-banner rounded dense class="settings-info-banner">
            Ralph Loop（迭代验证）在
            <strong>Agent</strong>
            标签页的「Ralph Loop」区块配置，与规划模式独立。
          </q-banner>
          <section class="adv-section">
            <div class="section-heading">
              <q-icon name="hub" color="primary" size="20px" />
              <div>
                <div class="text-subtitle2 text-weight-bold">通道绑定</div>
                <div class="text-caption text-grey-7">绑定 Agent 与消息入口通道及默认会话。</div>
              </div>
            </div>
            <div class="row q-col-gutter-md">
              <q-select
                v-model="selectedChannelId"
                class="col-12 col-md-6"
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
                class="col-12 col-md-6"
                dense
                outlined
                label="Chat ID"
                hint="外部平台的会话标识（如 chat_id / thread_id）"
                :disable="!selectedChannelId"
              />
            </div>
          </section>

          <section class="adv-section">
            <div class="section-heading">
              <q-icon name="folder_open" color="primary" size="20px" />
              <div>
                <div class="text-subtitle2 text-weight-bold">工作区</div>
                <div class="text-caption text-grey-7">Agent 运行时的文件系统根路径。</div>
              </div>
            </div>
            <q-input v-model="workspace" dense outlined label="工作区路径" hint="如 ~/.aranea/workspace/{agent_key}" />
          </section>

          <section class="adv-section">
            <div class="section-heading">
              <q-icon name="psychology" color="primary" size="20px" />
              <div>
                <div class="text-subtitle2 text-weight-bold">扩展思考（Reasoning）</div>
                <div class="text-caption text-grey-7">控制模型推理深度；仅当所选模型支持 Reasoning 时生效。</div>
              </div>
            </div>
            <div class="row q-col-gutter-md">
              <q-select
                v-model="reasoningMode"
                class="col-12 col-md-6"
                dense
                outlined
                emit-value
                map-options
                label="策略"
                :options="reasoningModeOptions"
              />
              <q-select
                v-model="reasoningLevel"
                class="col-12 col-md-6"
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

          <section class="adv-section">
            <div class="section-heading">
              <q-icon name="compress" color="primary" size="20px" />
              <div>
                <div class="text-subtitle2 text-weight-bold">上下文压缩</div>
                <div class="text-caption text-grey-7">长对话自动压缩与摘要策略。</div>
              </div>
            </div>
            <div class="row q-col-gutter-md">
              <q-toggle v-model="compactionEnabled" class="col-12 col-md-6" color="primary" label="启用上下文压缩" />
              <q-toggle v-model="sessionSummaryEnabled" class="col-12 col-md-6" color="primary" label="会话摘要" />
            </div>
          </section>

          <section class="adv-section">
            <div class="section-heading">
              <q-icon name="content_cut" color="primary" size="20px" />
              <div>
                <div class="text-subtitle2 text-weight-bold">上下文裁剪</div>
                <div class="text-caption text-grey-7">控制上下文窗口溢出时的裁剪行为。</div>
              </div>
            </div>
            <div class="row q-col-gutter-md">
              <q-select
                v-model="truncateStrategy"
                class="col-12 col-md-6"
                dense
                outlined
                emit-value
                map-options
                label="裁剪策略"
                :options="truncateStrategyOptions"
              />
              <q-input v-model.number="recentWindowTurns" class="col-12 col-md-6" dense outlined type="number" label="保留最近轮数" />
              <q-input v-model.number="recentWindowTokens" class="col-12 col-md-6" dense outlined type="number" label="保留最近 Token" />
              <q-input v-model.number="summaryKeepTurns" class="col-12 col-md-6" dense outlined type="number" label="摘要保留轮数" />
            </div>
          </section>

          <section class="adv-section">
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

      <q-card-actions align="right" class="dialog-footer">
        <q-btn flat rounded label="取消" v-close-popup />
        <q-btn color="primary" rounded unelevated label="保存" :loading="saving" @click="onSave" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { PlatformResource } from "../../features/platform/api";
import { useChannelsStore } from "../../stores/channels/index";

const channelsStore = useChannelsStore();

const props = defineProps<{
  open: boolean;
  saving: boolean;
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

const loadingChannels = ref(false);

const channelOptions = computed(() =>
  channelsStore.channels
    .filter((ch: PlatformResource) => ch.enabled)
    .map((ch: PlatformResource) => ({
      label: `${ch.name}（${ch.key}）`,
      value: ch.id,
      caption: ch.description
    }))
);

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

async function fetchChannels() {
  loadingChannels.value = true;
  try {
    await channelsStore.loadChannels();
  } catch {
    // error handled by store
  } finally {
    loadingChannels.value = false;
  }
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
    if (isOpen) {
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
      fetchChannels();
    }
  }
);
</script>

<style scoped>
.advanced-dialog {
  width: 720px;
  max-width: 94vw;
  max-height: 88vh;
  display: flex;
  flex-direction: column;
  border-radius: 24px;
  background: var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
  border: 1px solid var(--glass-border);
}

.dialog-header {
  padding: 20px 24px 12px;
  border-bottom: 1px solid var(--glass-border);
}

.dialog-body {
  flex: 1;
  min-height: 0;
}

.dialog-footer {
  padding: 12px 24px 20px;
  border-top: 1px solid var(--glass-border);
}

.adv-section {
  padding: 18px;
  border: 1px solid var(--glass-border);
  border-radius: 18px;
  background: var(--glass-elevated);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.section-heading {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin-bottom: 14px;
}

.adv-info-banner {
  border: 1px solid var(--glass-border);
  background: var(--glass-elevated);
  color: var(--color-text-secondary);
}

body.body--dark .advanced-dialog {
  background: var(--glass-surface);
  border-color: var(--glass-border);
}

body.body--dark .adv-section {
  background: var(--glass-elevated);
  border-color: var(--glass-border);
}
</style>
