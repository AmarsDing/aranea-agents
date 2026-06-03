<template>
  <q-card-section class="chat-composer q-px-md q-py-sm">
    <q-banner
      v-if="isAwaitingUser && awaitKind === AWAIT_KIND_TOOL_CONFIRM"
      rounded
      class="q-mb-sm app-banner-warning"
      dense
    >
      <template #avatar>
        <q-icon name="gpp_maybe" color="warning" />
      </template>
      <div class="text-body2">{{ t('chat.toolConfirmHint') }}</div>
      <div v-if="awaitToolKey" class="text-caption q-mt-xs">
        {{ t('chat.toolConfirmTool') }}: <code>{{ awaitToolKey }}</code>
      </div>
      <template #action>
        <q-btn
          flat
          dense
          no-caps
          color="negative"
          :label="t('chat.toolConfirmDeny')"
          class="q-mr-xs"
          @click="$emit('submit-tool-confirm', false)"
        />
        <q-btn
          flat
          dense
          no-caps
          color="primary"
          :label="t('chat.toolConfirmApprove')"
          @click="$emit('submit-tool-confirm', true)"
        />
      </template>
    </q-banner>
    <q-banner v-else-if="isAwaitingUser" rounded class="q-mb-sm app-banner-warning" dense>
      <template #avatar>
        <q-icon name="hourglass_top" color="amber-9" />
      </template>
      {{ t('chat.awaitingUserHint') }}
      <template #action>
        <q-btn
          flat
          dense
          no-caps
          color="primary"
          :label="t('chat.submitAwaitReply')"
          @click="$emit('submit-await-reply')"
        />
      </template>
    </q-banner>

    <q-banner v-if="contextPressureLevel === 'critical'" rounded class="q-mb-sm app-banner-warning" dense>
      <template #avatar>
        <q-icon name="warning" color="negative" />
      </template>
      <div class="text-body2">{{ t('chat.contextPressureCritical', '上下文窗口接近满载，回复可能被截断') }}</div>
      <template #action>
        <q-btn
          flat
          dense
          no-caps
          color="primary"
          :label="t('chat.contextPressureNewSession', '新会话')"
          @click="$emit('new-session')"
        />
      </template>
    </q-banner>
    <q-banner v-else-if="contextPressureLevel === 'warning'" rounded class="q-mb-sm app-banner-warning" dense>
      <template #avatar>
        <q-icon name="info" color="warning" />
      </template>
      {{ t('chat.contextPressureWarning', '上下文窗口使用率较高，建议开启新会话以获得更好效果') }}
    </q-banner>

    <div class="chat-composer-inner">
      <div v-if="attachments.length" class="chat-attachments row q-gutter-xs q-mb-sm">
        <div v-for="file in attachments" :key="file.id" class="chat-file-tile row items-center">
          <q-circular-progress
            v-if="file.progress < 1"
            :value="file.progress * 100"
            size="28px"
            :thickness="0.2"
            color="primary"
            class="q-mr-xs"
          />
          <q-icon v-else name="insert_drive_file" size="20px" class="q-mr-xs" color="primary" />
          <span class="ellipsis text-caption" style="max-width: 140px">{{ file.name }}</span
          ><q-tooltip>{{ file.name }}</q-tooltip>
          <q-btn
            icon="close"
            class="chat-file-tile__close"
            size="sm"
            round
            dense
            flat
            @click="$emit('remove-attachment', file.id)"
          />
        </div>
      </div>

      <ChatEnqueueMessage
        v-if="showEnqueue"
        :is-dark="isDark"
        :disabled="inputDisabled ?? sending"
        @enqueue="(text) => $emit('enqueue-message', text)"
      />

      <q-input
        :model-value="modelValue"
        filled
        class="chat-input"
        :label="t('chat.inputLabel')"
        type="textarea"
        autogrow
        :input-style="{ minHeight: '56px' }"
        :dark="isDark"
        :disable="isRunnerActive ? false : (inputDisabled ?? sending)"
        @keydown="onInputKeydown"
        @paste="handlePaste"
        @update:model-value="$emit('update:modelValue', String($event ?? ''))"
      />

      <div class="chat-toolbar q-mt-sm">
        <div class="chat-toolbar-fields row items-center no-wrap">
          <q-select
            :model-value="dialogMode"
            dense
            options-dense
            outlined
            :options="modeOptions"
            emit-value
            map-options
            :label="t('chat.dialogMode')"
            class="chat-toolbar-field chat-toolbar-field--mode"
            :dark="isDark"
            @update:model-value="$emit('update:dialogMode', String($event ?? ''))"
          />
          <q-select
            :model-value="modelProvider"
            dense
            options-dense
            outlined
            :options="providerOptions"
            emit-value
            map-options
            :label="t('chat.modelProvider')"
            class="chat-toolbar-field chat-toolbar-field--model"
            :dark="isDark"
            @update:model-value="$emit('update:modelProvider', String($event ?? ''))"
          >
            <template #option="scope">
              <q-item v-bind="scope.itemProps">
                <q-item-section>
                  <q-item-label>{{ scope.opt.label }}</q-item-label>
                  <q-item-label v-if="scope.opt.caption" caption>{{ scope.opt.caption }}</q-item-label>
                </q-item-section>
              </q-item>
            </template>
          </q-select>
          <q-select
            v-if="knowledgeBaseOptions?.length"
            :model-value="selectedKnowledgeBases ?? []"
            dense
            options-dense
            outlined
            multiple
            use-chips
            emit-value
            map-options
            :options="knowledgeBaseOptions"
            :label="t('chat.knowledgeBases')"
            class="chat-toolbar-field chat-toolbar-field--kb"
            :dark="isDark"
            @update:model-value="$emit('update:selectedKnowledgeBases', ($event as string[]) ?? [])"
          />
        </div>
        <div class="chat-toolbar-actions row items-center no-wrap">
          <ChatSessionArtifactsPanel
            v-if="sessionId"
            :session-id="sessionId"
            :items="sessionArtifacts ?? []"
            :loading="sessionArtifactsLoading"
            :is-dark="isDark"
            @open="$emit('open-artifact', $event)"
            @deleted="$emit('attachment-deleted', $event)"
          />
          <ChatBackgroundJobsPanel
            v-if="showBackgroundJobs"
            :session-id="sessionId"
            :agent-id="agentId"
            :refresh-nonce="jobsRefreshNonce"
            :is-dark="isDark"
            @focus-turn="$emit('focus-turn', $event)"
            @navigate="$emit('navigate', $event)"
            @cancel-job="$emit('cancel-job', $event)"
          />
          <q-btn
            dense
            unelevated
            outline
            color="primary"
            :disable="fileSupported === false"
            :aria-label="t('chat.fileImport')"
            class="chat-toolbar-btn chat-toolbar-btn--outline"
            @click="$emit('pick-file')"
          >
            <q-icon name="attach_file" size="18px" />
            <q-tooltip>{{
              fileSupported === false
                ? t('chat.fileNotSupported')
                : fileAccept
                  ? t('chat.limitedFileTypes')
                  : artifactMaxSizeHint()
            }}</q-tooltip>
          </q-btn>
          <q-btn
            dense
            unelevated
            outline
            color="primary"
            :aria-label="t('chat.voiceInput')"
            class="chat-toolbar-btn chat-toolbar-btn--outline"
            @click="$emit('voice')"
          >
            <q-icon name="mic" size="18px" />
          </q-btn>
          <q-btn
            v-if="sending || isRunnerActive"
            dense
            unelevated
            color="negative"
            :aria-label="t('chat.stop')"
            class="chat-toolbar-btn chat-toolbar-btn--filled"
            @click="$emit('stop')"
          >
            <q-icon name="stop" size="18px" />
          </q-btn>
          <q-btn
            v-else
            dense
            unelevated
            color="primary"
            :disable="!modelValue.trim()"
            :aria-label="t('chat.send')"
            class="chat-toolbar-btn chat-toolbar-btn--filled"
            @click="$emit('send')"
          >
            <q-icon name="send" size="18px" />
          </q-btn>
        </div>
      </div>
    </div>
  </q-card-section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import ChatEnqueueMessage from './ChatEnqueueMessage.vue';
import ChatSessionArtifactsPanel from './ChatSessionArtifactsPanel.vue';
import ChatBackgroundJobsPanel from './ChatBackgroundJobsPanel.vue';
import { AWAIT_KIND_TOOL_CONFIRM } from '../../features/chat/awaitConstants';
import type { ChatAttachment } from './types';
import type { ComposerUsageSnapshot } from '../../features/chat/composerUsageMetrics';
import type { ArtifactMeta } from '../../features/artifact/types';
import { artifactMaxSizeHint } from '../../features/artifact/limits';

type Option = { label: string; value: string; caption?: string };

const props = defineProps<{
  modelValue: string;
  attachments: ChatAttachment[];
  dialogMode: string;
  modelProvider: string;
  modeOptions: Option[];
  providerOptions: Option[];
  contextRatio: number;
  contextStatus?: string;
  usageSnapshot?: ComposerUsageSnapshot | null;
  /** @deprecated use usageSnapshot */
  sessionTotalTokens?: number | null;
  knowledgeBaseOptions?: Option[];
  selectedKnowledgeBases?: string[];
  isDark: boolean;
  sending?: boolean;
  inputDisabled?: boolean;
  isRunnerActive?: boolean;
  isAwaitingUser?: boolean;
  awaitKind?: string;
  awaitToolKey?: string;
  showEnqueue?: boolean;
  sessionId?: string;
  sessionArtifacts?: ArtifactMeta[];
  sessionArtifactsLoading?: boolean;
  fileSupported?: boolean;
  fileAccept?: string;
  showBackgroundJobs?: boolean;
  agentId?: string;
  jobsRefreshNonce?: number;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: string];
  'update:dialogMode': [value: string];
  'update:modelProvider': [value: string];
  'update:selectedKnowledgeBases': [value: string[]];
  'remove-attachment': [id: string];
  'pick-file': [];
  voice: [];
  send: [];
  stop: [];
  'enqueue-message': [content: string];
  'submit-await-reply': [];
  'submit-tool-confirm': [approved: boolean];
  'open-artifact': [id: string];
  'attachment-deleted': [id: string];
  'paste-file': [file: File];
  'focus-turn': [turnId: string];
  navigate: [route: { name: string; params: Record<string, string> }];
  'cancel-job': [job: { id: string; source: string }];
  'paste-unsupported': [];
  'new-session': [];
}>();

const { t } = useI18n();

const contextPressureLevel = computed<'warning' | 'critical' | null>(() => {
  const ratio = props.contextRatio ?? 0;
  const status = props.contextStatus?.trim();
  if (status === 'exceeded' || status === 'critical' || ratio >= 0.8) return 'critical';
  if (status === 'warning' || ratio >= 0.6) return 'warning';
  return null;
});

function handlePaste(event: ClipboardEvent) {
  const items = event.clipboardData?.items;
  if (!items || props.fileSupported === false) return;

  for (const item of Array.from(items)) {
    if (item.kind === 'file') {
      const file = item.getAsFile();
      if (!file) continue;
      const isImage =
        file.type?.toLowerCase().startsWith('image/') || /\.(png|jpe?g|gif|webp|bmp|svg|heic|heif)$/i.test(file.name);
      if (props.fileAccept && isImage) {
        event.preventDefault();
        emit('paste-unsupported');
        return;
      }
      event.preventDefault();
      emit('paste-file', file);
      return;
    }
  }
}

function onInputKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.shiftKey || event.isComposing || event.keyCode === 229) return;
  event.preventDefault();
  if (props.isRunnerActive) {
    const text = props.modelValue.trim();
    if (text) {
      emit('enqueue-message', text);
      emit('update:modelValue', '');
    }
  } else {
    emit('send');
  }
}
</script>
