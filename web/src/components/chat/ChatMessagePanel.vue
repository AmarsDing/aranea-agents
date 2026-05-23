<template>
  <q-card flat bordered class="col column no-wrap chat-mid-card" style="min-height: 0; border-radius: 18px">
    <q-banner v-if="wsReplaying" dense rounded class="q-mx-md q-mt-sm app-info-banner">
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
          {{ props.messages.length }} {{ t("chat.assistant") }} · {{ Math.round(props.contextRatio * 100) }}% ctx
        </div>
        <ChatRunnerStatus
          v-if="runStatus && runStatus !== 'idle' && runStatus !== 'completed' && runStatus !== 'cancelled' && runStatus !== 'failed'"
          :status="runStatus"
          :agent-name="runAgentName"
          :started-at="runStartedAt"
          :event-count="runEventCount"
          @cancel="emit('stop')"
        />
      </div>
      <q-btn flat round dense icon="bolt" aria-label="Session events" @click="emit('open-events')">
        <q-tooltip>会话事件</q-tooltip>
      </q-btn>
    </q-card-section>
    <ChatTeamMemberStrip v-if="isTeamSession" :members="teamMemberLanes" />
    <q-separator class="cream-sep" />
    <div :key="sessionTitle" class="chat-messages col column no-wrap" style="min-height: 0">
      <div
        v-if="!props.messages.length"
        ref="messagesScrollEl"
        class="col relative-position chat-messages__viewport"
        @click="onMessagesClick"
      >
        <div class="chat-empty-state column items-center justify-center">
          <div class="chat-empty-state__halo">
            <q-icon name="forum" size="38px" color="primary" />
          </div>
          <div class="chat-empty-state__title q-mt-md">{{ t("chat.emptyMessages") }}</div>
          <div class="chat-empty-state__hint text-caption q-mt-xs">{{ t("chat.inputLabel") }}</div>
        </div>
      </div>
      <q-virtual-scroll
        v-else-if="useVirtualMessageList"
        ref="virtualScrollRef"
        class="col chat-messages__viewport"
        style="min-height: 0"
        :items="props.messages"
        :virtual-scroll-item-size="virtualRowSize"
        :virtual-scroll-slice-size="24"
        :virtual-scroll-slice-ratio-before="1.5"
        :virtual-scroll-slice-ratio-after="1.5"
        v-slot="{ item, index }"
        @scroll="onMessagesScroll"
        @click="onMessagesClick"
      >
        <ChatMessageRow
          :message="item"
          :index="index"
          v-memo="[item.id, item.content_markdown, item.status, item.options_json, props.isDark, props.plannerKind]"
          :messages="props.messages"
          :is-dark="props.isDark"
          :is-team-session="props.isTeamSession"
          :planner-kind="props.plannerKind"
          :react-tool-link-index="props.reactToolLinkIndex"
          @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
        />
      </q-virtual-scroll>
      <div
        v-else
        ref="messagesScrollEl"
        class="col relative-position chat-messages__viewport"
        @scroll.passive="onMessagesScroll"
        @click="onMessagesClick"
      >
        <ChatMessageRow
          v-for="(message, idx) in props.messages"
          :key="message.id"
          v-memo="[message.id, message.content_markdown, message.status, message.options_json, props.isDark, props.plannerKind]"
          :message="message"
          :index="idx"
          :messages="props.messages"
          :is-dark="props.isDark"
          :is-team-session="props.isTeamSession"
          :planner-kind="props.plannerKind"
          :react-tool-link-index="props.reactToolLinkIndex"
          @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
        />
      </div>
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
        v-if="props.isAwaitingUser && props.awaitKind === AWAIT_KIND_TOOL_CONFIRM"
        rounded
        class="q-mb-sm app-banner-warning"
        dense
      >
        <template #avatar>
          <q-icon name="gpp_maybe" color="warning" />
        </template>
        <div class="text-body2">
          {{ t("chat.toolConfirmHint") }}
        </div>
        <div v-if="props.awaitToolKey" class="text-caption q-mt-xs">
          {{ t("chat.toolConfirmTool") }}: <code>{{ props.awaitToolKey }}</code>
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
      <q-banner
        v-else-if="props.isAwaitingUser"
        rounded
        class="q-mb-sm app-banner-warning"
        dense
      >
        <template #avatar>
          <q-icon name="hourglass_top" color="amber-9" />
        </template>
        {{ t("chat.awaitingUserHint") }}
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
      <div class="chat-composer-inner">
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

      <ChatEnqueueMessage
        v-if="showEnqueue"
        :is-dark="isDark"
        :disabled="inputDisabled ?? sending"
        @enqueue="(text) => emit('enqueue-message', text)"
      />

      <q-input
        :model-value="modelValue"
        filled
        class="chat-input"
        :label="t('chat.inputLabel')"
        type="textarea"
        autogrow
        :input-style="{ minHeight: '72px' }"
        :dark="isDark"
        :disable="inputDisabled ?? sending"
        @keydown="onInputKeydown"
        @update:model-value="$emit('update:modelValue', String($event ?? ''))"
      />

      <div class="chat-toolbar chat-toolbar-grid q-mt-sm">
        <q-select
          :model-value="dialogMode"
          dense
          options-dense
          outlined
          :options="modeOptions"
          emit-value
          map-options
          :label="t('chat.dialogMode')"
          class="chat-toolbar-field"
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
          class="chat-toolbar-field"
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
        <div class="chat-toolbar-actions row items-center no-wrap q-gutter-sm">
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
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import type { QVirtualScroll } from "quasar";
import ChatMessageRow from "./ChatMessageRow.vue";
import ChatRunnerStatus from "../../features/chat/components/ChatRunnerStatus.vue";
import ChatEnqueueMessage from "../../features/chat/components/ChatEnqueueMessage.vue";
import ChatTeamMemberStrip, { type TeamMemberLane } from "./ChatTeamMemberStrip.vue";
import type { RunStatusValue } from "../../features/chat/types";
import { useChatMessageRow } from "../../features/chat/useChatMessageRow";
import { AWAIT_KIND_TOOL_CONFIRM } from "../../features/chat/awaitConstants";
import {
  CHAT_VIRTUAL_ROW_ESTIMATE,
  CHAT_VIRTUAL_SCROLL_THRESHOLD,
} from "../../features/chat/chatListVirtual";
import { isActivityMessage } from "../../features/chat/mergeSessionMessages";
import type { A2UIUserActionPayload } from "../../features/chat/a2uiUserAction";
import type { Message, ReactToolLinkIndex } from "../../features/chat/types";
import type { ChatAttachment } from "./types";

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
  inputDisabled?: boolean;
  isAwaitingUser?: boolean;
  awaitKind?: string;
  awaitToolKey?: string;
  wsReplaying?: boolean;
  isTeamSession?: boolean;
  /** Active agent planner_kind (react / a2ui presentation). */
  plannerKind?: string;
  /** Session-level ReAct tool link index (O(n) once per message list). */
  reactToolLinkIndex: ReactToolLinkIndex;
  pendingMessages?: { id: string; content: string; status: string; created_at: string }[];
  runStatus?: RunStatusValue;
  runAgentName?: string;
  runStartedAt?: string;
  runEventCount?: number;
  showEnqueue?: boolean;
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
  "enqueue-message": [content: string];
  "cancel-pending": [pendingId: string];
  "update-pending": [pendingId: string, content: string];
  "submit-await-reply": [];
  "submit-tool-confirm": [approved: boolean];
  "open-events": [];
  "a2ui-user-action": [payload: A2UIUserActionPayload];
}>();

const { t } = useI18n();
const messagesRef = computed(() => props.messages);
const messageRow = useChatMessageRow(messagesRef);
const teamMemberLanes = computed((): TeamMemberLane[] => {
  if (!props.isTeamSession) return [];
  const lanes = new Map<string, TeamMemberLane>();
  for (const message of props.messages) {
    if (!messageRow.isTeamMember(message)) continue;
    const key = messageRow.messageIdentityKey(message);
    const meta = messageRow.teamMemberMeta(message);
    const label = meta?.name || meta?.agent_key || messageRow.displayMessageName(message);
    const streaming = message.status === "streaming" || message.status === "tool_running";
    const prev = lanes.get(key);
    lanes.set(key, {
      key,
      label,
      streaming: (prev?.streaming ?? false) || streaming
    });
  }
  return [...lanes.values()];
});
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

function maxScrollTop(el: HTMLElement): number {
  return Math.max(0, el.scrollHeight - el.clientHeight);
}

function distanceFromBottom(el: HTMLElement): number {
  return maxScrollTop(el) - el.scrollTop;
}

function activeScrollEl(): HTMLElement | null {
  if (useVirtualMessageList.value && virtualScrollRef.value) {
    return virtualScrollRef.value.$el as HTMLElement;
  }
  return messagesScrollEl.value;
}

/** Clamp scrollTop when layout reports an impossible position (blank message pane). */
function clampScrollTop(el: HTMLElement, preferBottom: boolean): void {
  const max = maxScrollTop(el);
  const top = el.scrollTop;
  if (!Number.isFinite(top) || top < 0 || top > max + 2) {
    el.scrollTop = preferBottom ? max : 0;
  }
}

function onMessagesScroll(event?: Event) {
  const el = (event?.target as HTMLElement | undefined) ?? activeScrollEl();
  if (!el) return;
  clampScrollTop(el, stickToBottom.value);
  const dist = distanceFromBottom(el);
  showScrollBtn.value = dist > 200;
  stickToBottom.value = dist <= SCROLL_BOTTOM_THRESHOLD;
}

function lastDialogueIndex(): number {
  for (let i = props.messages.length - 1; i >= 0; i--) {
    const m = props.messages[i]!;
    if (m.role === "user" && (m.content_markdown ?? "").trim()) return i;
    if (
      m.role === "assistant" &&
      !isActivityMessage(m) &&
      (m.content_markdown ?? "").trim()
    ) {
      return i;
    }
  }
  return Math.max(0, props.messages.length - 1);
}

async function scrollToLatestDialogue(smooth = false) {
  const idx = lastDialogueIndex();
  if (useVirtualMessageList.value && virtualScrollRef.value) {
    for (let attempt = 0; attempt < 4; attempt++) {
      await nextTick();
      if (virtualScrollRef.value) {
        virtualScrollRef.value.scrollTo(idx, smooth ? "start" : "start-force");
        stickToBottom.value = true;
        showScrollBtn.value = false;
        return;
      }
      await new Promise((resolve) => requestAnimationFrame(resolve));
    }
    return;
  }
  const el = messagesScrollEl.value;
  if (el) {
    const rows = el.querySelectorAll<HTMLElement>(".chat-q-message");
    const target = rows[idx];
    if (target) {
      target.scrollIntoView({ block: "end", behavior: smooth ? "smooth" : "auto" });
      stickToBottom.value = true;
      showScrollBtn.value = false;
      return;
    }
  }
  await scrollToBottom(smooth);
}

async function scrollToBottom(smooth = false) {
  if (useVirtualMessageList.value && props.messages.length > 0) {
    for (let attempt = 0; attempt < 6; attempt++) {
      await nextTick();
      if (virtualScrollRef.value) {
        virtualScrollRef.value.scrollTo(
          props.messages.length - 1,
          smooth ? "start" : "start-force"
        );
        stickToBottom.value = true;
        showScrollBtn.value = false;
        return;
      }
      await new Promise((resolve) => requestAnimationFrame(resolve));
    }
    return;
  }
  const el = messagesScrollEl.value;
  if (!el) return;
  clampScrollTop(el, true);
  const top = maxScrollTop(el);
  el.scrollTo({ top, behavior: smooth ? "smooth" : "auto" });
  stickToBottom.value = true;
  showScrollBtn.value = false;
}

/** Re-align scroll after session switch or first message hydrate (layout may settle late). */
async function alignMessageScroll(preferBottom: boolean) {
  for (let attempt = 0; attempt < 4; attempt++) {
    await nextTick();
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    const el = activeScrollEl();
    if (!el) continue;
    clampScrollTop(el, preferBottom);
    const top = preferBottom ? maxScrollTop(el) : 0;
    el.scrollTop = top;
    if (preferBottom && el.clientHeight > 0 && distanceFromBottom(el) <= SCROLL_BOTTOM_THRESHOLD) {
      stickToBottom.value = true;
      showScrollBtn.value = false;
      return;
    }
    if (!preferBottom && el.scrollTop <= 1) {
      return;
    }
  }
}

watch(
  () => props.sessionTitle,
  () => {
    stickToBottom.value = true;
    showScrollBtn.value = false;
    void alignMessageScroll(true);
  }
);

watch(
  () => props.messages.length,
  (len, prev) => {
    if (len === 0) return;
    if (prev === 0) {
      stickToBottom.value = true;
      void alignMessageScroll(true);
      return;
    }
    if (!stickToBottom.value) return;
    void scrollToLatestDialogue(false);
  }
);

onMounted(() => {
  if (props.messages.length > 0) {
    stickToBottom.value = true;
    void alignMessageScroll(true);
  }
});

watch(useVirtualMessageList, (enabled) => {
  if (enabled && props.messages.length > 0 && stickToBottom.value) {
    void scrollToLatestDialogue(false);
  }
});

let scrollStickRaf = 0;
watch(
  () => props.messages[props.messages.length - 1]?.content_markdown ?? "",
  () => {
    if (!stickToBottom.value) return;
    if (scrollStickRaf) return;
    scrollStickRaf = requestAnimationFrame(() => {
      scrollStickRaf = 0;
      void scrollToLatestDialogue(false);
    });
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
  if (scrollStickRaf) cancelAnimationFrame(scrollStickRaf);
});
</script>

