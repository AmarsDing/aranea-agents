<template>
  <q-card-section class="chat-composer">
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
        v-if="showEnqueue && !workspaceMode"
        :is-dark="isDark"
        :disabled="inputDisabled ?? sending"
        @enqueue="(text) => $emit('enqueue-message', text)"
      />

      <!-- 统一圆角卡片：附件 + 选择器 + 输入框 + 底部工具条 -->
      <div class="composer-card" :class="{ 'composer-card--dark': isDark, 'composer-card--compact': compactMode }">
        <!-- 精简模式切换按钮（右上角） -->
        <q-btn
          flat
          dense
          round
          size="sm"
          :icon="compactMode ? 'unfold_more' : 'unfold_less'"
          :aria-label="compactToggleLabel"
          class="composer-compact-toggle"
          @click="compactMode = !compactMode"
        >
          <q-tooltip>{{ compactToggleLabel }}</q-tooltip>
        </q-btn>

        <div class="composer-context-row" :class="{ 'row items-center q-gutter-xs': compactMode }">
          <!-- 已选 skill 标签（输入框上方，仅 ✕ 可删，防误删） -->
          <div v-if="selectedSkillEntries.length" class="chat-skill-chips row q-gutter-xs">
            <q-chip
              v-for="skill in selectedSkillEntries"
              :key="skill.slug"
              removable
              dense
              square
              class="chat-skill-chip"
              icon="auto_awesome"
              @remove="$emit('toggle-skill', skill.slug)"
            >
              {{ skill.name || skill.slug }}
              <q-tooltip>{{ skill.description || skill.slug }}</q-tooltip>
            </q-chip>
          </div>

          <!-- 已选附件始终可见，精简模式只收起配置。 -->
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
              <span class="ellipsis text-caption">{{ file.name }}</span>
              <q-tooltip>{{ file.name }}</q-tooltip>
              <q-btn
                icon="close"
                class="chat-file-tile__close"
                size="sm"
                round
                dense
                flat
                :aria-label="t('taskWorkspace.removeAttachment', { name: file.name })"
                @click="$emit('remove-attachment', file.id)"
              />
            </div>
          </div>
        </div>

        <!-- 顶部：模式 / 模型 / 知识库（精简模式隐藏） -->
        <div v-show="!compactMode" class="composer-top-bar row no-wrap items-center q-gutter-x-sm">
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
        </div>

        <!-- 听写状态条：录音中显示实时识别文本（M74 dictation） -->
        <div v-if="dictating" class="dictation-bar row items-center no-wrap">
          <span class="dictation-bar__dot" />
          <span class="dictation-bar__text ellipsis">{{ dictationPartial || t('chat.voiceDictationListening') }}</span>
        </div>

        <!-- 输入框 -->
        <q-input
          :model-value="modelValue"
          filled
          class="composer-input"
          :label="compactMode ? '' : t('chat.inputLabel')"
          :aria-label="t('taskWorkspace.inputName')"
          :placeholder="t(isRunnerActive ? 'taskWorkspace.followUpPlaceholder' : 'taskWorkspace.inputPlaceholder')"
          :dense="compactMode"
          :hide-bottom-space="compactMode"
          type="textarea"
          :autogrow="workspaceMode || !compactMode"
          :input-style="
            compactMode && !workspaceMode
              ? { minHeight: '32px', maxHeight: '32px', overflow: 'hidden', resize: 'none', padding: '0' }
              : { minHeight: workspaceMode ? '36px' : '56px', maxHeight: '200px' }
          "
          :dark="isDark"
          :disable="isRunnerActive ? false : (inputDisabled ?? sending)"
          @keydown="onInputKeydown"
          @paste="handlePaste"
          @update:model-value="$emit('update:modelValue', String($event ?? ''))"
        />

        <!-- 底部工具条：右侧操作按钮（精简模式隐藏，通过回车发送） -->
        <div v-if="queueFileBlocked || uploading" class="text-caption text-warning q-px-sm" role="status">
          {{ t(uploading ? 'taskWorkspace.uploadPending' : 'taskWorkspace.attachmentsWhileRunning') }}
        </div>
        <div v-if="draftSaveFailed" class="text-caption text-warning" role="status">
          {{ t('taskWorkspace.draftSaveFailed') }}
        </div>
        <div class="composer-bottom-bar row items-center justify-between no-wrap">
          <div v-if="compactMode" class="composer-model-summary text-caption text-cream-muted ellipsis">
            <div class="ellipsis">
              {{ providerOptions.find((option) => option.value === modelProvider)?.label || modelProvider }} ·
              {{ modeOptions.find((option) => option.value === dialogMode)?.label || dialogMode }}
            </div>
            <div class="composer-key-hint">
              {{ t(isRunnerActive ? 'taskWorkspace.followUpKeyboardHint' : 'taskWorkspace.keyboardHint') }}
            </div>
          </div>
          <!-- 右侧操作按钮 -->
          <div class="composer-actions row items-center no-wrap q-gutter-x-sm">
            <ChatSkillPicker
              v-if="skillCatalog?.length"
              :skills="skillCatalog"
              :selected-slugs="selectedSkillSlugs ?? []"
              @toggle="(slug: string) => $emit('toggle-skill', slug)"
              @clear="$emit('clear-skills')"
            />
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
              :outline="!dictating"
              :color="dictating ? 'negative' : 'accent'"
              :aria-label="dictating ? t('chat.voiceDictationStop') : t('chat.voiceInput')"
              class="composer-btn"
              :class="dictating ? 'composer-btn--filled composer-btn--recording' : 'composer-btn--outline'"
              @click="$emit('voice')"
            >
              <q-icon :name="dictating ? 'stop' : 'mic'" size="22px" />
              <q-tooltip v-if="dictating">{{ t('chat.voiceDictationStop') }}</q-tooltip>
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
              unelevated
              color="accent"
              :disable="submitDisabled"
              :loading="submitPending"
              :aria-label="t(isRunnerActive ? 'taskWorkspace.followUp' : 'chat.send')"
              :label="workspaceMode ? t(isRunnerActive ? 'taskWorkspace.followUp' : 'chat.send') : undefined"
              class="composer-btn composer-btn--filled"
              @click="$emit('send')"
            >
              <q-icon
                :name="isRunnerActive ? 'playlist_add' : 'send'"
                size="22px"
                :class="{ 'q-ml-xs': workspaceMode }"
              />
              <q-tooltip>{{ t(isRunnerActive ? 'taskWorkspace.followUp' : 'chat.send') }}</q-tooltip>
            </q-btn>
          </div>
        </div>
      </div>
    </div>
  </q-card-section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import ChatEnqueueMessage from './ChatEnqueueMessage.vue';
import ChatSkillPicker from './ChatSkillPicker.vue';
import type { ChatAttachment } from './types';
import type { SkillCatalogEntry } from '../../features/skills/types';
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
  isDark: boolean;
  sending?: boolean;
  submitPending?: boolean;
  inputDisabled?: boolean;
  isRunnerActive?: boolean;
  showEnqueue?: boolean;
  sessionId?: string;
  fileSupported?: boolean;
  fileAccept?: string;
  /** M74 听写中（麦克风录音态 UI）。 */
  dictating?: boolean;
  /** 听写识别中的部分文本（输入框上方实时字幕）。 */
  dictationPartial?: string;
  /** 当前 agent 可用 skill 目录（后端 skill.catalog 事件推送）。 */
  skillCatalog?: SkillCatalogEntry[];
  /** skill 选择器已选中的 slug 列表（chips 区展示）。 */
  selectedSkillSlugs?: string[];
  workspaceMode?: boolean;
  draftSaveFailed?: boolean;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: string];
  'update:dialogMode': [value: string];
  'update:modelProvider': [value: string];
  'remove-attachment': [id: string];
  'pick-file': [];
  voice: [];
  send: [];
  stop: [];
  'enqueue-message': [content: string];
  'paste-file': [file: File];
  'paste-unsupported': [];
  'new-session': [];
  'toggle-skill': [slug: string];
  'clear-skills': [];
}>();

const { t } = useI18n();

/** 选中 slug → catalog entry（chips 显示名称/简介；目录未到时回退 slug） */
const selectedSkillEntries = computed<SkillCatalogEntry[]>(() => {
  const slugs = props.selectedSkillSlugs ?? [];
  const catalog = props.skillCatalog ?? [];
  return slugs.map((slug) => catalog.find((s) => s.slug === slug) ?? { slug, name: '', description: '', tags: [] });
});

/** Compact settings only; selected context and primary actions remain visible. */
const compactMode = ref(Boolean(props.workspaceMode));
const uploading = computed(() => props.attachments.some((file) => file.progress < 1));
const queueFileBlocked = computed(() => Boolean(props.isRunnerActive && props.attachments.length));
const submitDisabled = computed(
  () =>
    props.submitPending ||
    (!props.modelValue.trim() && !props.selectedSkillSlugs?.length) ||
    uploading.value ||
    queueFileBlocked.value ||
    (!props.isRunnerActive && Boolean(props.inputDisabled ?? props.sending)),
);

/** 切换按钮的可访问性标签与提示文案（统一来源，避免模板中重复） */
const compactToggleLabel = computed(() =>
  compactMode.value ? t('chat.expandInput', '展开输入') : t('chat.compactInput', '精简输入'),
);

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
  if (!submitDisabled.value) emit('send');
}
</script>

<style scoped lang="sass">
/* 听写状态条（M74 dictation）：录音态红点 + 实时识别文本 */
.dictation-bar
  padding: 4px 10px
  border-radius: 10px
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)
  backdrop-filter: blur(var(--glass-blur-subtle))
  font-size: 12px
  color: var(--color-text-secondary)

.dictation-bar__dot
  flex: none
  width: 8px
  height: 8px
  border-radius: 50%
  margin-right: 8px
  background: var(--color-danger)
  animation: dictation-pulse 1.2s ease-in-out infinite

.dictation-bar__text
  min-width: 0

@keyframes dictation-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3

/* 统一圆角卡片 */
.composer-card
  position: relative
  border: 1px solid var(--glass-border)
  border-radius: 18px
  background: var(--glass-surface)
  backdrop-filter: blur(var(--glass-blur-default))
  padding: 12px 14px 10px
  display: flex
  flex-direction: column
  gap: 10px
  transition: box-shadow 0.2s ease

  &:focus-within
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-accent) 35%, transparent)

  &--dark
    background: var(--glass-surface)

  /* 展开模式：切换按钮浮动到右上角（仅非 compact 模式生效，避免 HMR 残留导致 compact 模式下覆盖输入框） */
  .composer-compact-toggle
    position: absolute
    top: 6px
    right: 6px
    z-index: 1

  &--compact
    padding: 8px 10px
    gap: 6px
    flex-direction: column
    align-items: stretch
    .composer-context-row
      padding-right: 32px
      max-height: 72px
      overflow-y: auto
    .chat-skill-chips, .chat-attachments
      margin: 0
    .chat-file-tile
      max-width: 180px

/* 精简模式切换按钮基础样式（默认静态，定位由 .composer-card 嵌套控制） */
.composer-compact-toggle
  flex-shrink: 0
  color: var(--color-text-secondary)

  &:hover
    color: var(--color-accent)

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

/* textarea 原生元素本身也保持透明、无边框；超出 maxHeight 时显示主题滚动条 */
.composer-card :deep(.composer-input .q-field__native),
.composer-card :deep(.composer-input .q-field__native:hover),
.composer-card :deep(.composer-input .q-field__native:focus),
.composer-card :deep(.composer-input .q-field__native:focus-visible)
  background: transparent !important
  border: none !important
  outline: none !important
  box-shadow: none !important
  /* autogrow 会将 overflow-y 设为 hidden，这里覆盖以在超出 maxHeight 时显示滚动条 */
  overflow-y: auto !important
  /* Firefox */
  scrollbar-width: thin
  scrollbar-color: var(--chat-scrollbar-thumb) transparent

/* WebKit 滚动条：日间 */
.composer-card :deep(.composer-input .q-field__native)::-webkit-scrollbar
  width: 8px

.composer-card :deep(.composer-input .q-field__native)::-webkit-scrollbar-track
  background: transparent

.composer-card :deep(.composer-input .q-field__native)::-webkit-scrollbar-thumb
  border-radius: 6px
  border: 2px solid transparent
  background-clip: content-box
  background-color: var(--chat-scrollbar-thumb)

.composer-card :deep(.composer-input .q-field__native)::-webkit-scrollbar-thumb:hover
  background-color: var(--chat-scrollbar-thumb-hover)

/* WebKit 滚动条：夜间 */
body.body--dark .composer-card :deep(.composer-input .q-field__native)
  scrollbar-color: var(--chat-scrollbar-thumb-dark) transparent

body.body--dark .composer-card :deep(.composer-input .q-field__native)::-webkit-scrollbar-thumb
  background-color: var(--chat-scrollbar-thumb-dark)

body.body--dark .composer-card :deep(.composer-input .q-field__native)::-webkit-scrollbar-thumb:hover
  background-color: var(--chat-scrollbar-thumb-hover-dark)

/* 底部工具条 */
.composer-bottom-bar
  gap: 8px
.composer-model-summary
  min-width: 0
  flex: 1
.composer-key-hint
  font-size: 11px
.composer-context-row:empty
  display: none
.composer-btn--filled.bg-accent
  color: var(--button-primary-text) !important

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
