<template>
  <q-card flat bordered class="col column chat-mid-card" style="min-height: 0; border-radius: 18px">
    <q-banner v-if="wsReplaying" dense rounded class="q-mx-md q-mt-sm bg-blue-1 text-dark">
      <template #avatar>
        <q-spinner-dots color="primary" size="20px" />
      </template>
      {{ t("chat.wsReplaying", "正在同步历史事件…") }}
    </q-banner>
    <q-card-section class="chat-message-header row items-center no-wrap q-px-md q-py-sm">
      <div class="chat-message-header__pulse" aria-hidden="true">
        <span class="chat-message-header__dot" />
      </div>
      <div class="col ellipsis">
        <div class="chat-message-header__title ellipsis">{{ sessionTitle }}</div>
        <div class="chat-message-header__subtitle text-caption ellipsis">
          {{ props.messages.length }} {{ t("chat.assistant") }} 路 {{ Math.round(props.contextRatio * 100) }}% ctx
        </div>
      </div>
    </q-card-section>
    <q-separator class="cream-sep" />
    <div
      ref="messagesScrollEl"
      class="chat-messages col scroll relative-position"
      @scroll.passive="onMessagesScroll"
      @click="onMessagesClick"
    >
      <div v-if="!props.messages.length" class="chat-empty-state column items-center justify-center">
        <div class="chat-empty-state__halo">
          <q-icon name="forum" size="38px" color="primary" />
        </div>
        <div class="chat-empty-state__title q-mt-md">{{ t("chat.emptyMessages") }}</div>
        <div class="chat-empty-state__hint text-caption q-mt-xs">{{ t("chat.inputLabel") }}</div>
      </div>
      <q-virtual-scroll
        v-else-if="useVirtualMessageList"
        ref="virtualScrollRef"
        scroll-target=".chat-messages"
        :items="props.messages"
        :virtual-scroll-item-size="virtualRowSize"
        :virtual-scroll-slice-size="24"
        v-slot="{ item, index }"
      >
        <ChatMessageRow
          :message="item"
          :index="index"
          :messages="props.messages"
          :is-dark="props.isDark"
          :is-team-session="props.isTeamSession"
        />
      </q-virtual-scroll>
      <ChatMessageRow
        v-for="(message, idx) in props.messages"
        v-else
        :key="message.id"
        :message="message"
        :index="idx"
        :messages="props.messages"
        :is-dark="props.isDark"
        :is-team-session="props.isTeamSession"
      />
      <div v-if="props.pendingMessages?.length" class="chat-pending-list">
        <div class="chat-pending-label">{{ t("chat.pendingQueue") }}</div>
        <div v-for="pm in props.pendingMessages" :key="pm.id" class="chat-pending-item">
          <div v-if="editingPendingId === pm.id" class="chat-pending-item__edit">
            <q-input
              v-model="editingPendingContent"
              dense
              outlined
              autogrow
              class="chat-pending-item__edit-input"
              :dark="props.isDark"
              @keydown.enter.prevent="confirmEditPending(pm.id)"
              @keydown.escape.prevent="cancelEditPending"
            />
            <q-btn
              dense
              flat
              round
              size="sm"
              icon="check"
              color="positive"
              class="chat-pending-item__edit-confirm"
              :aria-label="t('chat.confirmEdit')"
              @click="confirmEditPending(pm.id)"
            />
            <q-btn
              dense
              flat
              round
              size="sm"
              icon="close"
              color="negative"
              class="chat-pending-item__edit-cancel"
              :aria-label="t('chat.cancelEdit')"
              @click="cancelEditPending"
            />
          </div>
          <template v-else>
            <div class="chat-pending-item__content ellipsis">{{ pm.content }}</div>
            <div class="chat-pending-item__meta">
              <span class="chat-pending-item__status">{{ pm.status }}</span>
              <span class="chat-pending-item__time">{{ formatStamp(pm.created_at) }}</span>
              <q-btn
                dense
                flat
                round
                size="sm"
                icon="edit"
                color="primary"
                class="chat-pending-item__edit-btn"
                :aria-label="t('chat.editPending')"
                @click="startEditPending(pm)"
              />
              <q-btn
                dense
                flat
                round
                size="sm"
                icon="cancel"
                color="negative"
                class="chat-pending-item__cancel"
                :aria-label="t('chat.cancelPending')"
                @click="$emit('cancel-pending', pm.id)"
              />
            </div>
          </template>
        </div>
      </div>
      <transition name="chat-scroll-fade">
        <q-btn
          v-if="showScrollBtn"
          round
          unelevated
          color="primary"
          icon="arrow_downward"
          class="chat-scroll-bottom"
          aria-label="滚动到最新消息"
          @click="scrollToBottom(true)"
        />
      </transition>
    </div>

    <q-separator class="cream-sep" />
    <q-card-section class="chat-composer q-pa-sm q-pa-md-sm">
      <q-banner
        v-if="props.isAwaitingUser"
        rounded
        class="q-mb-sm bg-amber-1 text-dark"
        dense
      >
        <template #avatar>
          <q-icon name="hourglass_top" color="amber-9" />
        </template>
        {{ t("chat.awaitingUserHint", "Agent 正在等待你的回复，在下方输入后点击「提交回复」。") }}
        <template #action>
          <q-btn
            flat
            dense
            no-caps
            color="primary"
            :label="t('chat.submitAwaitReply', '提交回复')"
            @click="$emit('submit-await-reply')"
          />
        </template>
      </q-banner>
      <div v-if="attachments.length" class="chat-attachments row q-gutter-xs q-mb-sm">
        <div
          v-for="file in attachments"
          :key="file.id"
          class="chat-file-tile row items-center"
        >
          <q-circular-progress
            v-if="file.progress < 1"
            :value="file.progress * 100"
            size="28px"
            :thickness="0.2"
            color="primary"
            class="q-mr-xs"
          />
          <q-icon v-else name="insert_drive_file" size="20px" class="q-mr-xs" color="primary" />
          <span class="ellipsis text-caption" style="max-width: 56px">{{ file.name }}</span>
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

      <q-input
        :model-value="modelValue"
        filled
        class="chat-input"
        :label="t('chat.inputLabel')"
        type="textarea"
        autogrow
        :input-style="{ minHeight: '100px' }"
        :dark="isDark"
        :disable="sending"
        @keydown="onInputKeydown"
        @update:model-value="$emit('update:modelValue', String($event ?? ''))"
      />

      <div class="chat-toolbar row items-center q-col-gutter-sm q-mt-sm">
        <q-select
          :model-value="dialogMode"
          dense
          options-dense
          outlined
          :options="modeOptions"
          emit-value
          map-options
          :label="t('chat.dialogMode')"
          class="chat-toolbar-field col-12 col-md-4"
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
          class="chat-toolbar-field col-12 col-md-4"
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
        <div class="chat-toolbar-actions col-12 col-md-4 row items-center no-wrap q-gutter-sm">
          <div class="chat-context-pill row items-center no-wrap">
            <span class="text-caption text-no-wrap q-mr-sm">
              {{ t("chat.contextUse") }}
            </span>
            <q-circular-progress
              :value="contextRatio * 100"
              show-value
              size="34px"
              :thickness="0.2"
              color="primary"
            >
              <span class="text-caption">{{ Math.round(contextRatio * 100) }}%</span>
            </q-circular-progress>
          </div>
          <q-space class="gt-sm" />
          <q-btn
            round
            dense
            unelevated
            color="primary"
            :aria-label="t('chat.fileImport')"
            class="chat-icon-btn q-ml-sm"
            @click="$emit('pick-file')"
          >
            <q-icon name="attach_file" />
          </q-btn>
          <q-btn
            round
            dense
            unelevated
            outline
            color="primary"
            :aria-label="t('chat.voiceInput')"
            class="chat-icon-btn"
            @click="$emit('voice')"
          >
            <q-icon name="mic" />
          </q-btn>
          <q-btn
            v-if="sending"
            round
            dense
            unelevated
            color="negative"
            :aria-label="t('chat.stop')"
            class="chat-icon-btn chat-send-btn"
            @click="$emit('stop')"
          >
            <q-icon name="stop" />
          </q-btn>
          <q-btn
            v-else
            round
            dense
            unelevated
            color="primary"
            :aria-label="t('chat.send')"
            class="chat-icon-btn chat-send-btn"
            @click="$emit('send')"
          >
            <q-icon name="send" />
          </q-btn>
        </div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import type { QVirtualScroll } from "quasar";
import ChatMessageRow from "./ChatMessageRow.vue";
import {
  CHAT_VIRTUAL_ROW_ESTIMATE,
  CHAT_VIRTUAL_SCROLL_THRESHOLD,
} from "../../features/chat/chatListVirtual";
import type { ChatAttachment, Message } from "./types";

type Option = { label: string; value: string; caption?: string };

const props = defineProps<{
  modelValue: string;
  messages: Message[];
  attachments: ChatAttachment[];
  dialogMode: string;
  modelProvider: string;
  modeOptions: Option[];
  providerOptions: Option[];
  sessionTitle: string;
  contextRatio: number;
  isDark: boolean;
  sending?: boolean;
  isAwaitingUser?: boolean;
  wsReplaying?: boolean;
  isTeamSession?: boolean;
  pendingMessages?: { id: string; content: string; status: string; created_at: string }[];
}>();

const emit = defineEmits<{
  "update:modelValue": [value: string];
  "update:dialogMode": [value: string];
  "update:modelProvider": [value: string];
  "remove-attachment": [id: string];
  "pick-file": [];
  voice: [];
  send: [];
  stop: [];
  "cancel-pending": [pendingId: string];
  "update-pending": [pendingId: string, content: string];
  "submit-await-reply": [];
}>();

const { t } = useI18n();
const useVirtualMessageList = computed(() => props.messages.length >= CHAT_VIRTUAL_SCROLL_THRESHOLD);
const virtualRowSize = CHAT_VIRTUAL_ROW_ESTIMATE;
const virtualScrollRef = ref<QVirtualScroll | null>(null);

const editingPendingId = ref("");
const editingPendingContent = ref("");

function startEditPending(pm: { id: string; content: string }) {
  editingPendingId.value = pm.id;
  editingPendingContent.value = pm.content;
}

function confirmEditPending(pendingId: string) {
  const content = editingPendingContent.value.trim();
  if (!content) return;
  emit("update-pending", pendingId, content);
  editingPendingId.value = "";
  editingPendingContent.value = "";
}

function cancelEditPending() {
  editingPendingId.value = "";
  editingPendingContent.value = "";
}

function onInputKeydown(event: KeyboardEvent) {
  if (event.key !== "Enter" || event.shiftKey || event.isComposing) return;
  event.preventDefault();
  emit("send");
}

// ===== Scroll-to-bottom & auto-scroll =====
const messagesScrollEl = ref<HTMLElement | null>(null);
const showScrollBtn = ref(false);
const stickToBottom = ref(true);
const SCROLL_BOTTOM_THRESHOLD = 80;

function distanceFromBottom(el: HTMLElement): number {
  return el.scrollHeight - el.scrollTop - el.clientHeight;
}

function onMessagesScroll() {
  const el = messagesScrollEl.value;
  if (!el) return;
  const dist = distanceFromBottom(el);
  showScrollBtn.value = dist > 200;
  stickToBottom.value = dist <= SCROLL_BOTTOM_THRESHOLD;
}

function scrollToBottom(smooth = false) {
  if (useVirtualMessageList.value && virtualScrollRef.value && props.messages.length > 0) {
    virtualScrollRef.value.scrollTo(props.messages.length - 1, smooth ? "start" : "start-force");
    stickToBottom.value = true;
    showScrollBtn.value = false;
    return;
  }
  const el = messagesScrollEl.value;
  if (!el) return;
  el.scrollTo({ top: el.scrollHeight, behavior: smooth ? "smooth" : "auto" });
  stickToBottom.value = true;
  showScrollBtn.value = false;
}

watch(
  () => props.messages.length,
  () => {
    if (!stickToBottom.value) return;
    void nextTick(() => scrollToBottom(false));
  }
);

watch(
  () => props.messages[props.messages.length - 1]?.content_markdown ?? "",
  () => {
    if (!stickToBottom.value) return;
    void nextTick(() => scrollToBottom(false));
  }
);

// ===== Code copy: event delegation on messages container =====
let copyResetTimer: ReturnType<typeof setTimeout> | null = null;

function onMessagesClick(event: MouseEvent) {
  const target = event.target as HTMLElement | null;
  if (!target) return;
  const btn = target.closest<HTMLButtonElement>(".code-block__copy");
  if (!btn) return;
  event.preventDefault();
  const block = btn.closest<HTMLElement>(".code-block");
  const code = block?.querySelector<HTMLElement>("pre code")?.innerText ?? "";
  if (!code) return;
  const apply = () => {
    btn.classList.add("is-copied");
    const textEl = btn.querySelector<HTMLElement>(".code-block__copy-text");
    const original = textEl?.textContent ?? "复制";
    if (textEl) textEl.textContent = "已复制";
    if (copyResetTimer) clearTimeout(copyResetTimer);
    copyResetTimer = setTimeout(() => {
      btn.classList.remove("is-copied");
      if (textEl) textEl.textContent = original;
    }, 1400);
  };
  if (navigator.clipboard?.writeText) {
    void navigator.clipboard.writeText(code).then(apply).catch(() => fallbackCopy(code, apply));
  } else {
    fallbackCopy(code, apply);
  }
}

function fallbackCopy(text: string, onSuccess: () => void) {
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    document.execCommand("copy");
    document.body.removeChild(ta);
    onSuccess();
  } catch {
    /* swallow */
  }
}

onBeforeUnmount(() => {
  if (copyResetTimer) clearTimeout(copyResetTimer);
});
</script>

<style scoped lang="sass">
// ===== Design tokens =====
$msg-radius: 18px
$msg-radius-sm: 6px
$msg-shadow-sm: 0 1px 2px rgba(15, 23, 42, 0.04), 0 2px 8px rgba(15, 23, 42, 0.06)
$msg-shadow-md: 0 4px 14px rgba(15, 23, 42, 0.08), 0 1px 3px rgba(15, 23, 42, 0.04)
$msg-shadow-dark: 0 2px 8px rgba(0, 0, 0, 0.32), 0 8px 24px rgba(0, 0, 0, 0.18)
$msg-shadow-sent: 0 6px 18px rgba(99, 102, 241, 0.28), 0 1px 3px rgba(79, 70, 229, 0.22)
$msg-user-max: 46rem
$msg-opposite-gutter: 0px
$msg-edge-gutter: 12px
$msg-block-gap: 36px
$msg-continued-gap: 14px

// Soft canvas tints for subtle depth 鈥?feels less flat
$canvas-light: linear-gradient(180deg, rgba(248, 250, 252, 0.55) 0%, rgba(241, 245, 249, 0.0) 320px)
$canvas-dark: linear-gradient(180deg, rgba(15, 23, 42, 0.45) 0%, rgba(15, 23, 42, 0.0) 320px)

.chat-mid-card
  background-clip: padding-box

// ===== Header =====
.chat-message-header
  position: relative
  min-height: 52px
  padding-top: 10px
  padding-bottom: 10px
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.65) 0%, rgba(255, 255, 255, 0.32) 100%)
  backdrop-filter: blur(8px)
  -webkit-backdrop-filter: blur(8px)

:global(.body--dark) .chat-message-header
  background: linear-gradient(180deg, rgba(30, 41, 59, 0.6) 0%, rgba(15, 23, 42, 0.32) 100%)

.chat-message-header__pulse
  display: flex
  align-items: center
  justify-content: center
  width: 28px
  height: 28px
  margin-right: 10px
  border-radius: 50%
  background: rgba(233, 162, 59, 0.14)

.chat-message-header__dot
  width: 9px
  height: 9px
  border-radius: 50%
  background: var(--color-accent)
  box-shadow: 0 0 0 0 color-mix(in srgb, var(--color-accent) 55%, transparent)
  animation: chat-pulse 2.4s ease-in-out infinite

@keyframes chat-pulse
  0%
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--color-accent) 50%, transparent)
  70%
    box-shadow: 0 0 0 8px transparent
  100%
    box-shadow: 0 0 0 0 transparent

.chat-message-header__title
  color: var(--color-surface-solid)
  font-size: 15px
  font-weight: 800
  letter-spacing: 0.01em
  line-height: 1.25

.chat-message-header__subtitle
  margin-top: 2px
  color: rgba(71, 85, 105, 0.78)
  font-size: 11.5px
  font-weight: 500
  letter-spacing: 0.04em
  text-transform: uppercase

:global(.body--dark) .chat-message-header__title
  color: var(--color-text-primary)

:global(.body--dark) .chat-message-header__subtitle
  color: rgba(203, 213, 225, 0.7)

// ===== Messages canvas =====
.chat-messages
  min-width: 0
  max-width: 100%
  padding: 28px $msg-edge-gutter 36px
  background: $canvas-light
  overflow-x: hidden
  scroll-behavior: smooth
  box-sizing: border-box

  // Custom scrollbar 鈥?subtle but discoverable
  &::-webkit-scrollbar
    width: 10px
  &::-webkit-scrollbar-track
    background: transparent
  &::-webkit-scrollbar-thumb
    border-radius: 8px
    border: 2px solid transparent
    background-clip: content-box
    background-color: rgba(100, 116, 139, 0.28)
  &::-webkit-scrollbar-thumb:hover
    background-color: rgba(100, 116, 139, 0.55)

:global(.body--dark) .chat-messages
  background: $canvas-dark
  &::-webkit-scrollbar-thumb
    background-color: rgba(148, 163, 184, 0.22)
  &::-webkit-scrollbar-thumb:hover
    background-color: rgba(148, 163, 184, 0.45)

// ===== Empty state =====
.chat-empty-state
  min-height: 220px
  padding: 24px

.chat-empty-state__halo
  position: relative
  display: flex
  align-items: center
  justify-content: center
  width: 88px
  height: 88px
  border-radius: 50%
  background: radial-gradient(closest-side, rgba(99, 102, 241, 0.18) 0%, rgba(99, 102, 241, 0.04) 70%, rgba(99, 102, 241, 0) 100%)
  &::before
    content: ""
    position: absolute
    inset: 0
    border-radius: 50%
    border: 1px dashed rgba(99, 102, 241, 0.28)
    animation: chat-empty-spin 22s linear infinite

@keyframes chat-empty-spin
  to
    transform: rotate(360deg)

.chat-empty-state__title
  color: var(--color-text-primary)
  font-size: 15px
  font-weight: 700

.chat-empty-state__hint
  color: rgba(71, 85, 105, 0.7)

:global(.body--dark) .chat-empty-state__title
  color: var(--color-text-primary)

:global(.body--dark) .chat-empty-state__hint
  color: rgba(203, 213, 225, 0.7)

// ===== File tile =====
.chat-file-tile
  position: relative
  min-width: 30px
  min-height: 30px
  max-width: 120px
  padding: 2px 4px
  background: rgba(255, 255, 255, 0.78)
  border: 1px solid rgba(141, 110, 99, 0.2)
  border-radius: 6px
  transition: transform 0.18s ease, box-shadow 0.18s ease

  &:hover
    transform: translateY(-1px)
    box-shadow: 0 4px 12px rgba(15, 23, 42, 0.08)

  .chat-file-tile__close
    position: absolute
    top: -4px
    right: -4px
    opacity: 0
    transition: opacity 0.15s ease

  &:hover .chat-file-tile__close
    opacity: 1

// ===== Quasar QChatMessage layout =====
// 占满中间栏宽度，仅保留窄边距
.chat-q-message
  width: 100%
  max-width: 100%
  animation: chat-message-in 0.32s cubic-bezier(0.22, 1, 0.36, 1) both

  & + .chat-q-message
    margin-top: $msg-block-gap

  & + .chat-q-message--continued
    margin-top: $msg-continued-gap
  :deep(.q-message)
    min-width: 0
    max-width: 100%
    margin-bottom: 0
    padding-bottom: 0

  :deep(.q-message-container)
    min-width: 0
    width: 100%
    max-width: 100%
    column-gap: 12px
    box-sizing: border-box
    align-items: flex-start

  :deep(.q-message-avatar)
    margin-top: 2px
    flex-shrink: 0

  :deep(.q-message-received .q-message-container)
    padding-right: $msg-opposite-gutter

  :deep(.q-message-sent .q-message-container)
    padding-left: $msg-opposite-gutter

  // 鍐呭鍒楋細鍏佽鏀剁缉鍒?fit-content锛沘gent 姣旂敤鎴风暐瀹斤紝渚夸簬闀挎枃/琛ㄦ牸
  :deep(.col-grow)
    flex: 1 1 auto
    display: flex
    flex-direction: column
    min-width: 0
    max-width: 100%

  :deep(.q-message-sent .col-grow)
    align-items: flex-end
    flex: 0 1 auto
    margin-left: auto
    max-width: min($msg-user-max, 92%)

  :deep(.q-message-received .col-grow)
    align-items: flex-start
    max-width: 100%

  // Quasar 鎶婂灞傛皵娉?.q-message-text 褰撳鍣紝鎴戜滑鍐嶈嚜甯︿竴涓?inner 瀛?div .chat-message-bubble
  // 鍏抽敭锛氭妸 Quasar 鑷甫鐨?bg/color/padding 鍏ㄩ儴閲嶇疆鎴愰€忔槑缁ф壙锛岃鎴戜滑鐨?.chat-message-bubble 鎺ョ鎵€鏈夎瑙?  :deep(.q-message-text)
    width: auto
    max-width: 100%
    min-width: 0
    background: transparent !important
    background-color: transparent !important
    box-shadow: none !important
    padding: 0 !important
    border-radius: 0
    color: inherit !important
    border: none !important

  :deep(.q-message-text:before),
  :deep(.q-message-text:after)
    display: none !important

  :deep(.q-message-text-content)
    display: block
    min-width: 0
    width: 100%
    max-width: 100%
    background: transparent !important
    background-color: transparent !important
    color: inherit !important
    padding: 0 !important
    margin: 0 !important

// 连续消息：保留头像列占位，仅隐藏头像图形
.chat-q-message--continued
  :deep(.q-message-avatar)
    visibility: hidden

@keyframes chat-message-in
  0%
    opacity: 0
    transform: translateY(6px) scale(0.985)
  100%
    opacity: 1
    transform: translateY(0) scale(1)

// ===== Bubble (鎴戜滑鑷鐨?inner 鍗＄墖) =====
.chat-message-bubble
  position: relative
  width: 100%
  max-width: 100%
  min-width: 0
  padding: 18px 22px
  border-radius: $msg-radius
  font-size: 15px
  line-height: 1.8
  box-sizing: border-box
  transition: box-shadow 0.22s ease, transform 0.22s ease
  overflow: hidden
  overflow-wrap: anywhere

// 气泡昼夜材质见 app-global.sass「聊天消息气泡」
.chat-message-bubble--sent::after
  content: ""
  position: absolute
  inset: 0
  pointer-events: none
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.14) 0%, rgba(255, 255, 255, 0) 40%)
  border-radius: inherit

.chat-q-message--member
  margin-left: 8px
  padding-left: 6px
  border-left: 2px solid rgba(92, 107, 192, 0.35)

.chat-reasoning-details summary
  cursor: pointer
  color: var(--color-text-secondary, #64748b)

.chat-reasoning-details__body
  margin-top: 6px
  font-size: 13px
  opacity: 0.92

// 鍥㈤槦鎴愬憳姘旀场锛氬乏渚?4px 褰╄壊 accent 鏉★紙棰滆壊涓庡ご鍍忎竴鑷达級
.chat-message-bubble--member
  padding-left: 18px
  &::before
    content: ""
    position: absolute
    left: 0
    top: 0
    bottom: 0
    width: 4px
    background: var(--bubble-accent, var(--color-accent))
    border-radius: 4px 0 0 4px

.chat-message-bubble--tool
  border-style: dashed
  background-image: linear-gradient(180deg, rgba(14, 165, 233, 0.12), rgba(14, 165, 233, 0.04))

.chat-message-bubble--tool-running
  border-color: rgba(14, 165, 233, 0.42)
  box-shadow: 0 8px 24px rgba(14, 165, 233, 0.18), 0 1px 3px rgba(15, 23, 42, 0.06)

.chat-message-bubble--tool-failed
  border-color: rgba(239, 68, 68, 0.46)
  background-image: linear-gradient(180deg, rgba(239, 68, 68, 0.12), rgba(239, 68, 68, 0.04))

.chat-message-bubble--dark.chat-message-bubble--tool
  background-image: linear-gradient(180deg, rgba(14, 165, 233, 0.16), rgba(30, 41, 59, 0.92))
  border-color: rgba(56, 189, 248, 0.36)

.chat-message-bubble--dark.chat-message-bubble--tool-failed
  background-image: linear-gradient(180deg, rgba(239, 68, 68, 0.18), rgba(30, 41, 59, 0.92))
  border-color: rgba(248, 113, 113, 0.42)

// ===== Avatar =====
.message-avatar
  position: relative
  flex-shrink: 0
  font-weight: 700
  letter-spacing: 0.02em
  box-shadow: 0 8px 20px rgba(15, 23, 42, 0.18), 0 0 0 3px rgba(255, 255, 255, 0.92)
  border: none
  transition: transform 0.22s ease, box-shadow 0.22s ease

  &:hover
    transform: translateY(-1px) scale(1.04)

  &--user
    box-shadow: 0 8px 20px rgba(99, 102, 241, 0.30), 0 0 0 3px rgba(255, 255, 255, 0.95)

:global(.body--dark) .message-avatar
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.5), 0 0 0 3px rgba(30, 41, 59, 0.95)
  &--user
    box-shadow: 0 8px 20px rgba(99, 102, 241, 0.5), 0 0 0 3px rgba(30, 41, 59, 0.95)

.message-avatar__initials
  font-size: 14px
  line-height: 1.1

// ===== Name + Stamp row锛堝湪姘旀场涔嬪锛孮ChatMessage 鐨?#name 鎻掓Ы锛?=====
.message-meta-row
  display: flex
  align-items: baseline
  gap: 10px
  margin-bottom: 0
  padding: 0
  flex-wrap: wrap

.message-meta-row--sent
  justify-content: flex-end
  flex-direction: row-reverse

.message-name
  font-size: 13px
  font-weight: 650
  letter-spacing: 0.01em
  color: var(--color-text-primary)
  line-height: 1.25

.message-stamp
  font-size: 11px
  font-weight: 500
  letter-spacing: 0.03em
  color: var(--color-text-secondary)
  line-height: 1.25
  font-variant-numeric: tabular-nums
  opacity: 0.88

.message-send-tags
  display: block
  width: 100%
  margin-top: 8px
  text-align: right
  align-self: flex-end
  font-weight: 600
  letter-spacing: 0.01em
  line-height: 1.35
  opacity: 0.92

.message-send-tags--sent
  color: rgba(255, 255, 255, 0.82)

:global(.body--dark) .message-send-tags:not(.message-send-tags--sent)
  color: rgba(148, 163, 184, 0.95)

// ===== Streaming typing indicator =====
.chat-typing
  display: inline-flex
  align-items: center
  gap: 5px
  margin-top: 6px
  padding: 4px 0
  vertical-align: middle

  i
    width: 6px
    height: 6px
    border-radius: 50%
    background: currentColor
    opacity: 0.45
    animation: chat-typing-bounce 1.2s ease-in-out infinite

  i:nth-child(2)
    animation-delay: 0.18s
  i:nth-child(3)
    animation-delay: 0.36s

@keyframes chat-typing-bounce
  0%, 60%, 100%
    transform: translateY(0)
    opacity: 0.4
  30%
    transform: translateY(-4px)
    opacity: 1

// 流式强调条见 app-global（inset box-shadow）；成员左条保留 ::before
.chat-q-message--streaming.chat-q-message--member .chat-message-bubble::before
  animation: chat-stream-accent 1.6s ease-in-out infinite

@keyframes chat-stream-accent
  0%, 100%
    opacity: 0.55
  50%
    opacity: 1

// ===== Scroll-to-bottom button =====
.chat-scroll-bottom
  position: absolute
  right: 24px
  bottom: 18px
  z-index: 5
  width: 44px
  height: 44px
  box-shadow: 0 10px 24px rgba(15, 23, 42, 0.22), 0 2px 4px rgba(15, 23, 42, 0.08)

.chat-scroll-fade-enter-active,
.chat-scroll-fade-leave-active
  transition: opacity 0.18s ease, transform 0.22s cubic-bezier(0.22, 1, 0.36, 1)

.chat-scroll-fade-enter-from,
.chat-scroll-fade-leave-to
  opacity: 0
  transform: translateY(8px) scale(0.92)

// ===== 宸ュ叿璋冪敤缁撴灉锛氶粯璁ゆ姌鍙狅紝鐐瑰紑灞曞紑璇︽儏锛坮ead_file / shell 杈撳嚭绛夛級=====
.chat-tool-details
  width: 100%
  min-width: 0
  margin-top: 2px

.chat-tool-details__summary
  cursor: pointer
  list-style: none
  display: flex
  flex-wrap: wrap
  align-items: baseline
  gap: 8px 12px
  padding: 6px 4px
  border-radius: 10px
  user-select: none
  outline: none
  &::-webkit-details-marker
    display: none
  &:hover
    background: rgba(14, 165, 233, 0.09)

.chat-tool-details[open] > .chat-tool-details__summary
  margin-bottom: 4px

.chat-tool-details__summary-text
  flex: 1
  min-width: 0
  font-weight: 600
  font-size: 14px
  line-height: 1.45
  color: var(--color-text-primary)

.chat-tool-details__hint
  flex-shrink: 0
  letter-spacing: 0.02em
  &::after
    content: "灞曞紑璇︽儏"
    color: rgba(71, 85, 105, 0.92)

.chat-tool-details[open] .chat-tool-details__hint::after
  content: "鏀惰捣"

.chat-tool-details__body
  margin-top: 8px
  padding-top: 10px
  border-top: 1px dashed rgba(148, 163, 184, 0.45)

.chat-message-bubble--dark .chat-tool-details__summary-text
  color: var(--color-text-primary)

.chat-message-bubble--dark .chat-tool-details__hint::after
  color: rgba(148, 163, 184, 0.92)

.chat-message-bubble--dark .chat-tool-details__summary:hover
  background: rgba(56, 189, 248, 0.1)

.chat-message-bubble--dark .chat-tool-details__body
  border-top-color: rgba(148, 163, 184, 0.28)

// ===== Markdown content =====
.chat-message-content
  width: 100%
  max-width: 100%
  min-width: 0
  box-sizing: border-box
  font-size: 15px
  overflow-wrap: anywhere
  white-space: normal
  line-height: 1.78
  position: relative
  z-index: 1

  :deep(h1),
  :deep(h2),
  :deep(h3),
  :deep(h4),
  :deep(h5),
  :deep(h6)
    margin: 0.9em 0 0.45em
    color: var(--color-text-heading)
    font-weight: 700
    line-height: 1.28
    letter-spacing: -0.005em

  :deep(h1)
    font-size: 1.25rem
  :deep(h2)
    font-size: 1.125rem
  :deep(h3)
    font-size: 1.05rem
  :deep(h4),
  :deep(h5),
  :deep(h6)
    font-size: 1rem

  :deep(h1:first-child),
  :deep(h2:first-child),
  :deep(h3:first-child)
    margin-top: 0

  :deep(p)
    margin: 0 0 0.65em
  :deep(p:last-child)
    margin-bottom: 0
  :deep(p:first-child)
    margin-top: 0

  :deep(ul),
  :deep(ol)
    margin: 0.35em 0 0.8em
    padding-left: 1.35em
  :deep(li + li)
    margin-top: 0.2em

  :deep(a)
    color: var(--color-accent)
    text-decoration: none
    border-bottom: 1px solid rgba(233, 162, 59, 0.4)
    transition: color 0.15s ease, border-color 0.15s ease
  :deep(a:hover)
    color: var(--color-accent-hover)
    border-bottom-color: rgba(212, 140, 26, 0.85)

  :deep(table)
    display: block
    width: 100%
    max-width: 100%
    overflow-x: auto
    table-layout: auto
    margin: 0.75em 0
    border-collapse: collapse
    border: 1px solid var(--glass-border)
    border-radius: 12px
    background: color-mix(in srgb, var(--glass-surface) 92%, transparent)
    font-size: 13px
    line-height: 1.5
    box-shadow: none

  :deep(thead)
    background: color-mix(in srgb, var(--glass-surface) 88%, transparent)

  :deep(th),
  :deep(td)
    min-width: 96px
    padding: 9px 12px
    border: 1px solid rgba(100, 116, 139, 0.22)
    text-align: left
    vertical-align: top

  :deep(th)
    color: var(--color-text-primary)
    font-weight: 700
    letter-spacing: 0.01em
  :deep(td)
    color: var(--color-text-slate-700)

  // ===== Code block (with header: lang label + copy button) =====
  :deep(.code-block)
    width: 100%
    max-width: 100%
    min-width: 0
    margin: 0.85em 0
    border-radius: 12px
    overflow: hidden
    background: linear-gradient(180deg, var(--color-surface-solid) 0%, var(--canvas-base) 100%)
    box-shadow: inset 0 0 0 1px rgba(148, 163, 184, 0.16)
    text-align: left

  :deep(.code-block__header)
    display: flex
    align-items: center
    justify-content: space-between
    padding: 6px 12px
    background: rgba(15, 23, 42, 0.78)
    border-bottom: 1px solid rgba(148, 163, 184, 0.16)

  :deep(.code-block__lang)
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace
    font-size: 11.5px
    font-weight: 600
    letter-spacing: 0.06em
    text-transform: uppercase
    color: rgba(203, 213, 225, 0.78)

  :deep(.code-block__copy)
    display: inline-flex
    align-items: center
    gap: 5px
    padding: 4px 9px
    border: 1px solid rgba(148, 163, 184, 0.22)
    border-radius: 6px
    background: rgba(255, 255, 255, 0.04)
    color: rgba(226, 232, 240, 0.82)
    font-size: 11.5px
    font-weight: 600
    letter-spacing: 0.02em
    cursor: pointer
    transition: background 0.15s ease, border-color 0.15s ease, color 0.15s ease

  :deep(.code-block__copy:hover)
    background: rgba(99, 102, 241, 0.18)
    border-color: rgba(99, 102, 241, 0.45)
    color: var(--color-on-accent)

  :deep(.code-block__copy.is-copied)
    background: rgba(34, 197, 94, 0.18)
    border-color: rgba(34, 197, 94, 0.5)
    color: var(--color-accent-green-bright)

  :deep(.code-block__copy-icon)
    width: 12px
    height: 12px
    border-radius: 2px
    border: 1.5px solid currentColor
    box-shadow: 2px -2px 0 0 rgba(255, 255, 255, 0.25), -1px 1px 0 0 currentColor

  :deep(.code-block pre)
    max-width: 100%
    min-width: 0
    margin: 0
    padding: 14px 16px
    background: transparent
    box-shadow: none
    border-radius: 0
    color: var(--color-text-dark)
    line-height: 1.6
    font-size: 13.5px
    overflow-x: auto
    white-space: pre

  // 鍏滃簳锛氫竾涓€鏈蛋 fence renderer 鐨?<pre>
  :deep(pre)
    width: 100%
    max-width: 100%
    min-width: 0
    overflow-x: auto
    margin: 0.8em 0
    padding: 14px 16px
    border-radius: 12px
    background: linear-gradient(180deg, var(--color-surface-solid) 0%, var(--canvas-base) 100%)
    color: var(--color-text-dark)
    line-height: 1.6
    font-size: 13.5px
    box-shadow: inset 0 0 0 1px rgba(148, 163, 184, 0.12)

  :deep(code)
    padding: 0.14em 0.42em
    border-radius: 6px
    background: rgba(15, 23, 42, 0.07)
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace
    font-size: 0.92em

  :deep(pre code)
    display: block
    min-width: 0
    padding: 0
    background: transparent
    font-size: inherit
    white-space: pre

  :deep(blockquote)
    margin: 0.8em 0
    padding: 0.4em 0.95em
    border-left: 3px solid rgba(233, 162, 59, 0.55)
    background: rgba(233, 162, 59, 0.08)
    border-radius: 0 8px 8px 0
    color: var(--color-text-secondary)

  :deep(hr)
    margin: 1.1em 0
    border: 0
    border-top: 1px solid rgba(100, 116, 139, 0.22)

  :deep(img)
    max-width: 100%
    border-radius: 10px

// ===== Sent (user) variant =====
.chat-message-content--sent
  text-align: right

:global(body:not(.body--dark)) .chat-message-content--sent
  :deep(h1),
  :deep(h2),
  :deep(h3),
  :deep(h4),
  :deep(h5),
  :deep(h6),
  :deep(p),
  :deep(li)
    color: var(--color-on-accent)

  :deep(a)
    color: var(--color-on-accent)
    border-bottom-color: rgba(255, 255, 255, 0.45)

  :deep(code)
    background: rgba(255, 255, 255, 0.18)
    color: var(--color-on-accent)

  :deep(.code-block)
    background: rgba(15, 23, 42, 0.62)
    box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.16)

  :deep(blockquote)
    background: rgba(255, 255, 255, 0.1)
    border-left-color: rgba(255, 255, 255, 0.65)
    color: rgba(255, 255, 255, 0.92)

  :deep(table)
    background: rgba(255, 255, 255, 0.1)
    border-color: rgba(255, 255, 255, 0.28)
  :deep(th),
  :deep(td)
    color: var(--color-on-accent)
    border-color: rgba(255, 255, 255, 0.22)

// ===== Dark theme content =====
.chat-message-content--dark
  color: var(--color-text-primary)

  :deep(h1),
  :deep(h2),
  :deep(h3),
  :deep(h4),
  :deep(h5),
  :deep(h6)
    color: var(--color-text-heading)

  :deep(p),
  :deep(li),
  :deep(span),
  :deep(strong),
  :deep(em),
  :deep(del)
    color: inherit

  :deep(a)
    color: var(--color-link)
    border-bottom-color: rgba(147, 197, 253, 0.45)
  :deep(a:hover)
    color: var(--color-accent-blue-light)
    border-bottom-color: rgba(191, 219, 254, 0.85)

  :deep(table)
    background: rgba(15, 23, 42, 0.55)
    border-color: rgba(203, 213, 225, 0.22)
  :deep(thead)
    background: rgba(0, 229, 255, 0.1)
  :deep(th)
    color: var(--color-text-heading)
  :deep(td)
    color: var(--color-text-primary)
  :deep(th),
  :deep(td)
    border-color: rgba(148, 163, 184, 0.2)

  :deep(code)
    background: rgba(148, 163, 184, 0.14)
    color: var(--color-text-dark)

  :deep(blockquote)
    background: rgba(0, 229, 255, 0.08)
    color: var(--color-text-secondary)
    border-left-color: rgba(0, 229, 255, 0.55)

  :deep(hr)
    border-top-color: rgba(203, 213, 225, 0.18)

// ===== Responsive =====
@media (max-width: 1280px)
  .chat-messages
    padding-left: 16px
    padding-right: 16px

@media (max-width: 900px)
  .chat-messages
    padding-left: 28px
    padding-right: 28px

  .chat-message-bubble
    padding: 16px 18px

@media (max-width: 599px)
  .chat-messages
    padding-left: 14px
    padding-right: 14px
    padding-top: 18px
    padding-bottom: 24px

  .chat-q-message
    & + .chat-q-message
      margin-top: 28px
    & + .chat-q-message--continued
      margin-top: 12px
    :deep(.q-message-received .q-message-container),
    :deep(.q-message-sent .q-message-container)
      padding-left: 0
      padding-right: 0
      column-gap: 10px
    :deep(.col-grow)
      max-width: 100%

  .chat-message-bubble
    padding: 14px 16px
    font-size: 14.5px
    line-height: 1.75

  .chat-message-bubble--member
    padding-left: 14px

  .message-meta-row
    gap: 8px
  .message-name
    font-size: 13.5px
  .message-stamp
    font-size: 11px

  .chat-scroll-bottom
    right: 12px
    bottom: 12px
    width: 40px
    height: 40px

// Reduced motion
@media (prefers-reduced-motion: reduce)
  .chat-q-message,
  .chat-message-header__dot,
  .chat-empty-state__halo::before,
  .chat-typing i,
  .chat-q-message--streaming.chat-q-message--member .chat-message-bubble::before
    animation: none !important
  .message-avatar,
  .chat-message-bubble
    transition: none

.chat-pending-list
  margin: 8px 16px
  padding: 8px 12px
  border-radius: 12px
  background: rgba(251, 191, 36, 0.08)
  border: 1px solid rgba(251, 191, 36, 0.22)

.chat-pending-label
  font-size: 12px
  font-weight: 600
  color: var(--color-accent-amber-dark)
  margin-bottom: 6px
  text-transform: uppercase
  letter-spacing: 0.04em

:global(.body--dark) .chat-pending-label
  color: var(--color-accent-amber)

.chat-pending-item
  padding: 6px 8px
  border-radius: 8px
  background: rgba(255, 255, 255, 0.55)
  margin-bottom: 4px
  &:last-child
    margin-bottom: 0

:global(.body--dark) .chat-pending-item
  background: rgba(15, 23, 42, 0.35)

.chat-pending-item__content
  font-size: 13px
  color: var(--color-text-primary)
  line-height: 1.4

:global(.body--dark) .chat-pending-item__content
  color: var(--color-text-dark)

.chat-pending-item__meta
  display: flex
  gap: 8px
  margin-top: 2px
  font-size: 11px
  color: var(--color-text-tertiary)

:global(.body--dark) .chat-pending-item__meta
  color: var(--color-text-tertiary)

.chat-pending-item__status
  font-weight: 600
  text-transform: uppercase

.chat-pending-item__cancel
  margin-left: auto
  opacity: 0.6
  &:hover
    opacity: 1

.chat-pending-item__edit-btn
  opacity: 0.6
  &:hover
    opacity: 1

.chat-pending-item__edit
  display: flex
  align-items: flex-start
  gap: 4px

.chat-pending-item__edit-input
  flex: 1

.chat-pending-item__edit-confirm,
.chat-pending-item__edit-cancel
  margin-top: 4px
</style>
