<template>
  <q-card-section class="chat-composer" style="padding: 8px var(--chat-edge-gutter, 12px)">
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
          color="accent"
          :label="t('chat.toolConfirmApprove')"
          @click="$emit('submit-tool-confirm', true)"
        />
      </template>
    </q-banner>
    <q-banner v-else-if="isAwaitingUser" rounded class="q-mb-sm app-banner-warning" dense>
      <template #avatar>
        <q-icon name="hourglass_top" color="warning" />
      </template>
      {{ t('chat.awaitingUserHint') }}
      <template #action>
        <q-btn
          flat
          dense
          no-caps
          color="accent"
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
          color="accent"
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
      <ChatEnqueueMessage
        v-if="showEnqueue"
        :is-dark="isDark"
        :disabled="inputDisabled ?? sending"
        @enqueue="(text) => $emit('enqueue-message', text)"
      />

      <!-- 统一圆角卡片：附件 + 选择器 + 输入框 + 底部工具条 -->
      <div class="composer-card" :class="{ 'composer-card--dark': isDark }">
        <!-- 附件缩略图（输入框上方） -->
        <div v-if="attachments.length" class="chat-attachments row q-gutter-xs">
          <div v-for="file in attachments" :key="file.id" class="chat-file-tile row items-center">
            <q-circular-progress
              v-if="file.progress < 1"
              :value="file.progress * 100"
              size="28px"
              :thickness="0.2"
              color="accent"
              class="q-mr-xs"
            />
            <q-icon v-else name="insert_drive_file" size="20px" class="q-mr-xs" color="accent" />
            <span class="ellipsis text-caption" style="max-width: 140px">{{ file.name }}</span>
            <q-tooltip>{{ file.name }}</q-tooltip>
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

        <!-- 顶部：模式 / 模型 / 知识库 -->
        <div class="composer-top-bar row no-wrap items-center q-gutter-x-sm">
          <q-select
            :model-value="dialogMode"
            dense
            options-dense
            outlined
            :options="modeOptions"
            emit-value
            map-options
            :label="t('chat.dialogMode')"
            class="composer-field composer-field--mode"
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
            class="composer-field composer-field--model"
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
            class="composer-field composer-field--kb"
            :dark="isDark"
            @update:model-value="$emit('update:selectedKnowledgeBases', ($event as string[]) ?? [])"
          />
        </div>

        <!-- 输入框 -->
        <q-input
          :model-value="modelValue"
          filled
          class="composer-input"
          :label="t('chat.inputLabel')"
          type="textarea"
          autogrow
          :input-style="{ minHeight: '56px', maxHeight: '200px' }"
          :dark="isDark"
          :disable="isRunnerActive ? false : (inputDisabled ?? sending)"
          @keydown="onInputKeydown"
          @paste="handlePaste"
          @update:model-value="$emit('update:modelValue', String($event ?? ''))"
        />

        <!-- 底部工具条：右侧操作按钮 -->
        <div class="composer-bottom-bar row items-center justify-end no-wrap">
          <!-- 右侧操作按钮 -->
          <div class="composer-actions row items-center no-wrap q-gutter-x-sm">
            <span class="composer-btn-wrapper">
              <q-btn
                unelevated
                outline
                color="accent"
                :disable="fileSupported === false"
                :aria-label="t('chat.fileImport')"
                class="composer-btn composer-btn--outline"
                @click="$emit('pick-file')"
              >
                <q-icon name="attach_file" size="22px" />
              </q-btn>
              <q-tooltip anchor="top middle" self="bottom middle">{{
                fileSupported === false
                  ? t('chat.fileNotSupported')
                  : fileAccept
                    ? t('chat.limitedFileTypes')
                    : artifactMaxSizeHint()
              }}</q-tooltip>
            </span>
            <q-btn
              unelevated
              outline
              color="accent"
              :aria-label="t('chat.voiceInput')"
              class="composer-btn composer-btn--outline"
              @click="$emit('voice')"
            >
              <q-icon name="mic" size="22px" />
            </q-btn>
            <q-btn
              v-if="sending || isRunnerActive"
              unelevated
              color="negative"
              :aria-label="t('chat.stop')"
              class="composer-btn composer-btn--filled"
              @click="$emit('stop')"
            >
              <q-icon name="stop" size="22px" />
            </q-btn>
            <q-btn
              v-else
              unelevated
              color="accent"
              :disable="!modelValue.trim()"
              :aria-label="t('chat.send')"
              class="composer-btn composer-btn--filled"
              @click="$emit('send')"
            >
              <q-icon name="send" size="22px" />
            </q-btn>
          </div>
        </div>
      </div>
    </div>
  </q-card-section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import ChatEnqueueMessage from './ChatEnqueueMessage.vue';
import { AWAIT_KIND_TOOL_CONFIRM } from '../../features/chat/awaitConstants';
import type { ChatAttachment } from './types';
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
  fileSupported?: boolean;
  fileAccept?: string;
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
  'paste-file': [file: File];
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
      if (props.fileAccept && isImage && !props.fileAccept.includes('image/') && !props.fileAccept.includes('*')) {
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
    }
  } else {
    emit('send');
  }
}
</script>

<style scoped lang="sass">
/* 统一圆角卡片 */
.composer-card
  border: 1px solid var(--glass-border)
  border-radius: 18px
  background: var(--glass-surface)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))
  padding: 12px 14px 10px
  display: flex
  flex-direction: column
  gap: 10px
  transition: box-shadow 0.2s ease

  &:focus-within
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-accent) 35%, transparent)

  &--dark
    background: var(--glass-surface)

/* 顶部选择器行 */
.composer-top-bar
  flex-wrap: nowrap
  overflow: hidden

.composer-field
  flex: 1 1 auto
  min-width: 0

  &--mode
    flex: 0 1 130px

  &--model
    flex: 0 1 180px

  &--kb
    flex: 1 1 auto

/* 输入框：保持 hover/focus 状态下背景色与默认一致，不变化 */
.composer-card :deep(.composer-input .q-field__control)
  background: transparent !important
  border-radius: 12px
  box-shadow: none !important

/* 覆盖所有 Quasar filled 状态（默认/hover/focus/highlighted 及组合）的 ::before/::after 伪元素 */
.composer-card :deep(.composer-input .q-field__control::before),
.composer-card :deep(.composer-input .q-field__control::after),
.composer-card :deep(.composer-input:hover .q-field__control::before),
.composer-card :deep(.composer-input:hover .q-field__control::after),
.composer-card :deep(.composer-input.q-field--filled .q-field__control::before),
.composer-card :deep(.composer-input.q-field--filled .q-field__control::after),
.composer-card :deep(.composer-input.q-field--filled:hover .q-field__control::before),
.composer-card :deep(.composer-input.q-field--filled:hover .q-field__control::after),
.composer-card :deep(.composer-input.q-field--focused .q-field__control::before),
.composer-card :deep(.composer-input.q-field--focused .q-field__control::after),
.composer-card :deep(.composer-input.q-field--highlighted .q-field__control::before),
.composer-card :deep(.composer-input.q-field--highlighted .q-field__control::after),
.composer-card :deep(.composer-input.q-field--filled.q-field--focused .q-field__control::before),
.composer-card :deep(.composer-input.q-field--filled.q-field--focused .q-field__control::after),
.composer-card :deep(.composer-input.q-field--filled.q-field--highlighted .q-field__control::before),
.composer-card :deep(.composer-input.q-field--filled.q-field--highlighted .q-field__control::after)
  background: transparent !important
  display: none !important

/* textarea 原生元素本身也保持透明、无边框 */
.composer-card :deep(.composer-input .q-field__native),
.composer-card :deep(.composer-input .q-field__native:hover),
.composer-card :deep(.composer-input .q-field__native:focus),
.composer-card :deep(.composer-input .q-field__native:focus-visible)
  background: transparent !important
  border: none !important
  outline: none !important
  box-shadow: none !important

/* 底部工具条 */
.composer-bottom-bar
  gap: 8px

/* 右侧按钮 */
.composer-btn-wrapper
  display: inline-flex

.composer-btn
  border-radius: 12px
  min-height: 40px
  min-width: 40px
  padding: 8px 10px

  &--outline
    border-width: 1.5px

  &--filled
    box-shadow: 0 2px 6px color-mix(in srgb, var(--color-accent) 28%, transparent)

    &:not(:disabled):hover
      box-shadow: 0 4px 10px color-mix(in srgb, var(--color-accent) 38%, transparent)

@media (max-width: 899px)
  .composer-top-bar
    flex-wrap: wrap

  .composer-field
    flex: 1 1 100%

  .composer-bottom-bar
    flex-wrap: wrap
    gap: 6px

  .composer-actions
    flex: 1 1 100%
    justify-content: flex-end
</style>
