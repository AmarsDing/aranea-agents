<template>
  <q-card flat bordered class="col column chat-mid-card" style="min-height: 0; border-radius: 18px">
    <q-card-section class="chat-message-header row items-center no-wrap q-px-md q-py-sm">
      <div class="chat-message-header__pulse" aria-hidden="true">
        <span class="chat-message-header__dot" />
      </div>
      <div class="col ellipsis">
        <div class="chat-message-header__title ellipsis">{{ sessionTitle }}</div>
        <div class="chat-message-header__subtitle text-caption ellipsis">
          {{ props.messages.length }} {{ t("chat.assistant") }} · {{ Math.round(props.contextRatio * 100) }}% ctx
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
      <q-chat-message
        v-for="(message, idx) in props.messages"
        :key="message.id"
        class="chat-q-message"
        :class="{
          'chat-q-message--continued': isContinued(idx),
          'chat-q-message--streaming': isStreaming(message),
          'chat-q-message--member': isTeamMember(message)
        }"
        :sent="message.role === 'user'"
        size="grow"
      >
        <template #avatar>
          <q-avatar
            v-if="message.role === 'user'"
            :size="messageAvatarSize"
            color="white"
            text-color="primary"
            class="message-avatar message-avatar--user self-start"
            rounded
            icon="person"
            :aria-label="displayMessageName(message)"
          />
          <q-avatar
            v-else
            :size="messageAvatarSize"
            :color="messageAvatarColor(message)"
            text-color="white"
            class="message-avatar self-start"
            :aria-label="displayMessageName(message)"
          >
            <resolved-avatar-img
              v-if="shouldRenderAgentAvatarImage(messageAvatarRawIcon(message))"
              :icon="messageAvatarRawIcon(message)"
              :alt="displayMessageName(message)"
            />
            <q-icon v-else-if="messageAvatarIcon(message)" :name="messageAvatarIcon(message)" :size="messageAvatarIconSize" />
            <span v-else class="message-avatar__initials">{{ messageAvatarInitials(message) }}</span>
          </q-avatar>
        </template>
        <div
          class="chat-message-bubble"
          :class="{
            'chat-message-bubble--sent': message.role === 'user',
            'chat-message-bubble--received': message.role !== 'user',
            'chat-message-bubble--dark': props.isDark,
            'chat-message-bubble--member': isTeamMember(message),
            'chat-message-bubble--tool': isToolEventMessage(message),
            'chat-message-bubble--tool-running': message.status === 'tool_running',
            'chat-message-bubble--tool-failed': message.status === 'tool_failed'
          }"
          :style="bubbleAccentStyle(message)"
        >
          <div class="message-meta-row" :class="{ 'message-meta-row--sent': message.role === 'user' }">
            <span class="message-name">{{ displayMessageName(message) }}</span>
            <span class="message-stamp">{{ formatStamp(message.created_at) }}</span>
          </div>
          <details
            v-if="isCollapsibleToolDetail(message)"
            class="chat-tool-details"
          >
            <summary class="chat-tool-details__summary">
              <span class="chat-tool-details__summary-text">{{ toolCollapseSummary(message) }}</span>
              <span class="chat-tool-details__hint text-caption" aria-hidden="true" />
            </summary>
            <div
              class="chat-message-content chat-tool-details__body"
              :class="{
                'chat-message-content--sent': message.role === 'user',
                'chat-message-content--dark': message.role !== 'user' && props.isDark
              }"
              v-html="renderMarkdown(toolCollapseDetail(message))"
            />
          </details>
          <div
            v-else
            class="chat-message-content"
            :class="{
              'chat-message-content--sent': message.role === 'user',
              'chat-message-content--dark': message.role !== 'user' && props.isDark
            }"
            v-html="isStreaming(message) ? renderStreamingMarkdown(message.content_markdown) : renderMarkdown(message.content_markdown)"
          />
          <div
            v-if="message.role !== 'user' && message.status === 'error' && assistantErrorDetail(message)"
            class="text-caption text-negative q-mt-xs chat-assistant-error"
          >
            {{ assistantErrorDetail(message) }}
          </div>
          <div
            v-if="message.role === 'user'"
            class="message-send-tags message-send-tags--sent text-caption"
          >
            {{ userSendTagLine(message) }}
          </div>
          <span v-if="isStreaming(message)" class="chat-typing" aria-label="正在输入">
            <i /><i /><i />
          </span>
        </div>
      </q-chat-message>
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
import DOMPurify from "dompurify";
import MarkdownIt from "markdown-it";
import { nextTick, onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import ResolvedAvatarImg from "../avatar/ResolvedAvatarImg.vue";
import { isAvatarAssetRef, shouldRenderAgentAvatarImage } from "../../features/avatar/iconModel";
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
}>();

const { t } = useI18n();
const messageAvatarSize = "44px";
const messageAvatarIconSize = "24px";

const editingPendingId = ref("");
const editingPendingContent = ref("");

/**
 * 头像调色板：name 给 QAvatar 用（Quasar 颜色），hex 给气泡 accent 条用。
 * 选用低饱和、长时间观看不刺眼的色阶。
 */
const AVATAR_PALETTE = [
  { name: "indigo", hex: "#5c6bc0" },
  { name: "cyan", hex: "#26c6da" },
  { name: "purple", hex: "#ab47bc" },
  { name: "teal", hex: "#26a69a" },
  { name: "deep-purple", hex: "#7e57c2" },
  { name: "deep-orange", hex: "#ff7043" },
  { name: "blue", hex: "#42a5f5" },
  { name: "pink", hex: "#ec407a" },
  { name: "green", hex: "#66bb6a" },
  { name: "amber", hex: "#ffa726" }
] as const;

type TeamMemberMessageMeta = {
  agent_id?: string;
  agent_key?: string;
  name?: string;
  role?: string;
  icon?: string;
};

type AgentMessageMeta = {
  agent_id?: string;
  agent_key?: string;
  name?: string;
  icon?: string;
};

type ToolMessageMeta = {
  id?: string;
  status?: string;
  tool_name?: string;
  tool_label?: string;
};

const markdown = new MarkdownIt({
  breaks: true,
  html: false,
  linkify: true
});
markdown.enable(["table", "strikethrough"]);

// 自定义代码块：增加 header（语言标签 + 复制按钮），按钮通过事件代理处理
markdown.renderer.rules.fence = (tokens, idx) => {
  const token = tokens[idx]!;
  const info = (token.info || "").trim();
  const lang = info ? info.split(/\s+/)[0]! : "";
  const langLabel = lang || "code";
  const safeCode = markdown.utils.escapeHtml(token.content);
  const codeClass = lang ? ` class="language-${markdown.utils.escapeHtml(lang)}"` : "";
  return `<div class="code-block">
    <div class="code-block__header">
      <span class="code-block__lang">${markdown.utils.escapeHtml(langLabel)}</span>
      <button type="button" class="code-block__copy" aria-label="复制代码">
        <span class="code-block__copy-icon" aria-hidden="true"></span>
        <span class="code-block__copy-text">复制</span>
      </button>
    </div>
    <pre><code${codeClass}>${safeCode}</code></pre>
  </div>`;
};

function formatStamp(iso: string) {
  if (!iso) return "";
  try {
    const d = new Date(iso);
    const now = new Date();
    const sameDay = d.toDateString() === now.toDateString();
    if (sameDay) {
      return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
    }
    const diffDays = Math.floor((now.getTime() - d.getTime()) / 86_400_000);
    if (diffDays < 7) {
      return d.toLocaleString(undefined, {
        weekday: "short",
        hour: "2-digit",
        minute: "2-digit"
      });
    }
    return d.toLocaleString(undefined, {
      month: "short",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit"
    });
  } catch {
    return iso;
  }
}

function renderMarkdown(content: string) {
  return DOMPurify.sanitize(markdown.render(content || ""), {
    ADD_TAGS: ["button"],
    ADD_ATTR: ["type", "aria-label", "aria-hidden"]
  });
}

function closeOpenFences(src: string): string {
  let count = 0;
  for (const line of src.split("\n")) {
    if (/^\s*```/.test(line)) count++;
  }
  if (count % 2 !== 0) return src + "\n```";
  return src;
}

function renderStreamingMarkdown(content: string) {
  const patched = closeOpenFences(content || "");
  return DOMPurify.sanitize(markdown.render(patched), {
    ADD_TAGS: ["button"],
    ADD_ATTR: ["type", "aria-label", "aria-hidden"]
  });
}

function displayMessageName(message: Message) {
  if (message.role === "user") return t("chat.me");
  const member = teamMemberMeta(message);
  if (member?.name) {
    return member.role ? `${member.name} (${member.role})` : member.name;
  }
  const agent = agentMeta(message);
  if (agent?.name) return agent.name;
  if (message.model_name?.startsWith("team/")) {
    const [, role, name] = message.model_name.split("/");
    if (name && role) return `${name} (${role})`;
    if (name) return name;
  }
  return t("chat.assistant");
}

function messageIdentityKey(message: Message): string {
  if (message.role === "user") return "user";
  const meta = teamMemberMeta(message) ?? agentMeta(message);
  if (meta?.agent_id) return meta.agent_id;
  if (meta?.agent_key) return meta.agent_key;
  if (message.model_name?.trim()) return message.model_name;
  return message.id || "assistant";
}

function assistantErrorDetail(message: Message): string {
  const raw = message.error_message?.trim();
  if (raw) return raw;
  const body = message.content_markdown?.trim() || "";
  if (body === "对话生成失败。") {
    return "未返回详细错误，请查看用量事件或后端日志。";
  }
  return "";
}

function userSendTagLine(message: Message): string {
  let agentLabel = "—";
  let ctx = "0%";
  let intentKind = "";
  try {
    const raw = JSON.parse(message.options_json || "{}") as {
      agent?: { name?: string; display_name?: string };
      send_meta?: { context_pct?: number };
      intent_artifact?: { intent_kind?: string };
    };
    const n = raw.agent?.name || raw.agent?.display_name;
    if (n) agentLabel = n;
    if (typeof raw.send_meta?.context_pct === "number") {
      ctx = `${Math.round(raw.send_meta.context_pct)}%`;
    }
    if (raw.intent_artifact?.intent_kind) {
      intentKind = raw.intent_artifact.intent_kind;
    }
  } catch {
    /* ignore */
  }
  const parts: string[] = [agentLabel, `${ctx} CTX`];
  if (intentKind) parts.push(intentKind);
  const st = message.status?.trim();
  if (st && st !== "ok") parts.push(st);
  const err = message.error_message?.trim();
  if (err) parts.push(err);
  return parts.join(" · ");
}

function isTeamMember(message: Message): boolean {
  return message.role !== "user" && (Boolean(teamMemberMeta(message)) || Boolean(message.model_name?.startsWith("team/")));
}

function isStreaming(message: Message): boolean {
  return message.status === "streaming" || message.status === "tool_running";
}

function isContinued(idx: number): boolean {
  if (idx <= 0) return false;
  const cur = props.messages[idx];
  const prev = props.messages[idx - 1];
  if (!cur || !prev) return false;
  if (prev.role !== cur.role) return false;
  return messageIdentityKey(prev) === messageIdentityKey(cur);
}

function paletteIndex(message: Message): number {
  const key = messageIdentityKey(message);
  let h = 0;
  for (let i = 0; i < key.length; i += 1) h = (h * 31 + key.charCodeAt(i)) >>> 0;
  return h % AVATAR_PALETTE.length;
}

function messageAvatarColor(message: Message): string {
  return AVATAR_PALETTE[paletteIndex(message)]?.name || "indigo";
}

function memberAccentHex(message: Message): string {
  return AVATAR_PALETTE[paletteIndex(message)]?.hex || "#5c6bc0";
}

function bubbleAccentStyle(message: Message): Record<string, string> | undefined {
  if (!isTeamMember(message)) return undefined;
  return { "--bubble-accent": memberAccentHex(message) };
}

function teamMemberMeta(message: Message): TeamMemberMessageMeta | null {
  try {
    const raw = JSON.parse(message.options_json || "{}") as { team_member?: TeamMemberMessageMeta };
    return raw.team_member ?? null;
  } catch {
    return null;
  }
}

function agentMeta(message: Message): AgentMessageMeta | null {
  try {
    const raw = JSON.parse(message.options_json || "{}") as { agent?: AgentMessageMeta };
    return raw.agent ?? null;
  } catch {
    return null;
  }
}

function toolEventMeta(message: Message): ToolMessageMeta | null {
  try {
    const raw = JSON.parse(message.options_json || "{}") as { tool_event?: ToolMessageMeta };
    return raw.tool_event ?? null;
  } catch {
    return null;
  }
}

function isToolEventMessage(message: Message): boolean {
  return Boolean(toolEventMeta(message)) || message.status.startsWith("tool_");
}

/** 首行作为摘要，其余（含 read_file 大段代码等）放入可折叠区。 */
function toolCollapseParts(message: Message): { summary: string; detail: string } {
  const md = message.content_markdown?.trim() ?? "";
  const nl = md.indexOf("\n");
  if (nl === -1) {
    return { summary: toolCollapsePlainLine(md), detail: "" };
  }
  const first = md.slice(0, nl).trim();
  const rest = md.slice(nl + 1).trim();
  return { summary: toolCollapsePlainLine(first), detail: rest };
}

function toolCollapsePlainLine(s: string): string {
  let t = s.replace(/\*\*/g, "").replace(/`/g, "").trim();
  if (t.length > 220) t = `${t.slice(0, 220)}…`;
  return t || "工具";
}

function toolCollapseSummary(message: Message): string {
  return toolCollapseParts(message).summary;
}

function toolCollapseDetail(message: Message): string {
  return toolCollapseParts(message).detail;
}

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

/** 工具「进行中」保持单行展开；有后续段落/代码块时默认折叠详情。 */
function isCollapsibleToolDetail(message: Message): boolean {
  if (!isToolEventMessage(message)) return false;
  if (message.status === "tool_running") return false;
  return toolCollapseParts(message).detail.length > 0;
}

function messageAvatarRawIcon(message: Message): string {
  return teamMemberMeta(message)?.icon || agentMeta(message)?.icon || "";
}

function messageAvatarIcon(message: Message): string {
  if (message.role === "user") return "person";
  const icon = messageAvatarRawIcon(message);
  if (icon && !isAvatarAssetRef(icon)) return icon;
  if (isAvatarAssetRef(icon)) return "";
  return isTeamMember(message) ? "" : "smart_toy";
}

function messageAvatarInitials(message: Message): string {
  const raw = displayMessageName(message);
  const compact = raw.replace(/[()（）]/g, " ").replace(/\s+/g, " ").trim();
  if (!compact) return "…";
  const parts = compact.split(" ").filter(Boolean);
  if (parts.length >= 2) {
    const a = parts[0]!.slice(0, 1);
    const b = parts[1]!.slice(0, 1);
    return (a + b).toUpperCase();
  }
  const w = parts[0] || compact;
  if (/[\u4e00-\u9fff]/.test(w) && w.length >= 2) return w.slice(0, 2);
  if (/[\u4e00-\u9fff]/.test(w)) return w.slice(0, 1);
  return w.length <= 2 ? w.toUpperCase() : w.slice(0, 2).toUpperCase();
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

// 流式增量到达时（最后一条消息内容增长）也保持贴底
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
$msg-opposite-gutter: 150px
$msg-edge-gutter: 80px

// Soft canvas tints for subtle depth — feels less flat
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
  background: rgba(34, 197, 94, 0.12)

.chat-message-header__dot
  width: 9px
  height: 9px
  border-radius: 50%
  background: #22c55e
  box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.6)
  animation: chat-pulse 2.4s ease-in-out infinite

@keyframes chat-pulse
  0%
    box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.55)
  70%
    box-shadow: 0 0 0 8px rgba(34, 197, 94, 0)
  100%
    box-shadow: 0 0 0 0 rgba(34, 197, 94, 0)

.chat-message-header__title
  color: #0f172a
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
  color: #f8fafc

:global(.body--dark) .chat-message-header__subtitle
  color: rgba(203, 213, 225, 0.7)

// ===== Messages canvas =====
.chat-messages
  min-width: 0
  max-width: 100%
  padding: 32px $msg-edge-gutter 40px
  background: $canvas-light
  overflow-x: hidden
  scroll-behavior: smooth

  // Custom scrollbar — subtle but discoverable
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
  color: #1e293b
  font-size: 15px
  font-weight: 700

.chat-empty-state__hint
  color: rgba(71, 85, 105, 0.7)

:global(.body--dark) .chat-empty-state__title
  color: #f1f5f9

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
// size="grow" + 内容自适应宽度（fit-content + max-width 上限），短消息不再撑满
.chat-q-message
  width: 100%
  animation: chat-message-in 0.32s cubic-bezier(0.22, 1, 0.36, 1) both

  & + .chat-q-message
    margin-top: 36px // 不同发送者间距大

  & + .chat-q-message--continued
    margin-top: 6px // 同一发送者连续消息紧贴

  :deep(.q-message)
    min-width: 0
    max-width: 100%
    margin-bottom: 0
    padding-bottom: 0

  :deep(.q-message-container)
    min-width: 0
    width: 100%
    max-width: 100%
    column-gap: 14px
    box-sizing: border-box
    align-items: flex-start

  :deep(.q-message-received .q-message-container)
    padding-right: $msg-opposite-gutter

  :deep(.q-message-sent .q-message-container)
    padding-left: $msg-opposite-gutter

  // 内容列：允许收缩到 fit-content；agent 比用户略宽，便于长文/表格
  :deep(.col-grow)
    flex: 0 1 auto
    display: flex
    flex-direction: column
    min-width: 0

  :deep(.q-message-sent .col-grow)
    align-items: flex-end
    margin-left: auto
    max-width: min(720px, 100%)

  :deep(.q-message-received .col-grow)
    align-items: flex-start
    max-width: min(1000px, 100%)

  // Quasar 把外层气泡 .q-message-text 当容器，我们再自带一个 inner 子 div .chat-message-bubble
  // 关键：把 Quasar 自带的 bg/color/padding 全部重置成透明继承，让我们的 .chat-message-bubble 接管所有视觉
  :deep(.q-message-text)
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

// 隐藏连续消息中的头像（避免列堆叠重复头像，依然预留头像列宽度）
.chat-q-message--continued
  :deep(.q-message-avatar)
    visibility: hidden
    height: 0
    margin-top: 0
    margin-bottom: 0

@keyframes chat-message-in
  0%
    opacity: 0
    transform: translateY(6px) scale(0.985)
  100%
    opacity: 1
    transform: translateY(0) scale(1)

// ===== Bubble (我们自管的 inner 卡片) =====
.chat-message-bubble
  position: relative
  width: fit-content
  max-width: 100%
  min-width: 0
  padding: 12px 16px
  border-radius: $msg-radius
  font-size: 15px
  line-height: 1.78
  box-sizing: border-box
  transition: box-shadow 0.22s ease, transform 0.22s ease
  overflow: hidden
  overflow-wrap: anywhere

.chat-message-bubble--received
  background: #ffffff
  color: #0f172a
  border: 1px solid rgba(148, 163, 184, 0.22)
  box-shadow: $msg-shadow-md
  background-image: linear-gradient(180deg, rgba(255, 255, 255, 1), rgba(248, 250, 252, 0.92))

.chat-message-bubble--sent
  color: #ffffff
  border: 1px solid rgba(99, 102, 241, 0.45)
  background: linear-gradient(135deg, #4f46e5 0%, #6366f1 50%, #818cf8 100%)
  box-shadow: $msg-shadow-sent
  // 顶部柔和高光
  &::after
    content: ""
    position: absolute
    inset: 0
    pointer-events: none
    background: linear-gradient(180deg, rgba(255, 255, 255, 0.16) 0%, rgba(255, 255, 255, 0) 38%)
    border-radius: inherit

// 团队成员气泡：左侧 4px 彩色 accent 条（颜色与头像一致）
.chat-message-bubble--member
  padding-left: 18px
  &::before
    content: ""
    position: absolute
    left: 0
    top: 0
    bottom: 0
    width: 4px
    background: var(--bubble-accent, #6366f1)
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

.chat-q-message:hover .chat-message-bubble--received
  box-shadow: 0 6px 22px rgba(15, 23, 42, 0.10), 0 2px 4px rgba(15, 23, 42, 0.05)

.chat-q-message:hover .chat-message-bubble--sent
  box-shadow: 0 8px 24px rgba(99, 102, 241, 0.36), 0 2px 4px rgba(79, 70, 229, 0.24)

// Dark mode（由父组件 props.isDark 直接控制 .chat-message-bubble--dark，避免依赖全局 body--dark）
.chat-message-bubble--dark.chat-message-bubble--received
  background: #1e293b
  background-image: linear-gradient(180deg, rgba(71, 85, 105, 0.55) 0%, rgba(30, 41, 59, 0.92) 100%)
  color: #f1f5f9
  border-color: rgba(148, 163, 184, 0.18)
  box-shadow: $msg-shadow-dark

.chat-message-bubble--dark.chat-message-bubble--sent
  background: linear-gradient(135deg, #3730a3 0%, #4338ca 55%, #4f46e5 100%)
  border-color: rgba(99, 102, 241, 0.6)
  color: #f8fafc

.chat-q-message:hover .chat-message-bubble--dark.chat-message-bubble--received
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.55), 0 2px 6px rgba(0, 0, 0, 0.3)

.chat-q-message:hover .chat-message-bubble--dark.chat-message-bubble--sent
  box-shadow: 0 10px 30px rgba(99, 102, 241, 0.45), 0 2px 8px rgba(79, 70, 229, 0.3)

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

// ===== Name + Stamp row（在气泡之外，QChatMessage 的 #name 插槽） =====
.message-meta-row
  display: flex
  align-items: baseline
  gap: 10px
  margin-bottom: 10px
  padding: 0
  flex-wrap: wrap

.message-meta-row--sent
  justify-content: flex-end
  flex-direction: row-reverse

.message-name
  font-size: 14px
  font-weight: 700
  letter-spacing: 0.01em
  color: #0f172a
  line-height: 1.2

.message-stamp
  font-size: 11.5px
  font-weight: 500
  letter-spacing: 0.02em
  color: rgba(71, 85, 105, 0.7)
  line-height: 1.2
  font-variant-numeric: tabular-nums

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

:global(.body--dark) .message-name
  color: #f1f5f9
:global(.body--dark) .message-stamp
  color: rgba(203, 213, 225, 0.62)

.chat-message-bubble--sent .message-name,
.chat-message-bubble--sent .message-stamp
  color: rgba(255, 255, 255, 0.94)

.chat-message-bubble--sent .message-stamp
  color: rgba(255, 255, 255, 0.72)

.chat-message-bubble--dark.chat-message-bubble--received .message-name
  color: #f8fafc

.chat-message-bubble--dark.chat-message-bubble--received .message-stamp
  color: rgba(203, 213, 225, 0.68)

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

// 流式中的气泡左边缘加柔和闪烁高光
.chat-q-message--streaming .chat-message-bubble
  &::before
    background: linear-gradient(180deg, var(--bubble-accent, #6366f1) 0%, var(--bubble-accent, #6366f1) 100%) !important
    animation: chat-stream-glow 1.6s ease-in-out infinite

@keyframes chat-stream-glow
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

// ===== 工具调用结果：默认折叠，点开展开详情（read_file / shell 输出等）=====
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
  color: #0f172a

.chat-tool-details__hint
  flex-shrink: 0
  letter-spacing: 0.02em
  &::after
    content: "展开详情"
    color: rgba(71, 85, 105, 0.92)

.chat-tool-details[open] .chat-tool-details__hint::after
  content: "收起"

.chat-tool-details__body
  margin-top: 8px
  padding-top: 10px
  border-top: 1px dashed rgba(148, 163, 184, 0.45)

.chat-message-bubble--dark .chat-tool-details__summary-text
  color: #f1f5f9

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
  word-break: break-word
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
    color: #0f172a
    font-weight: 700
    line-height: 1.28
    letter-spacing: -0.005em

  :deep(h1)
    font-size: 24px
  :deep(h2)
    font-size: 21px
  :deep(h3)
    font-size: 18px
  :deep(h4),
  :deep(h5),
  :deep(h6)
    font-size: 16px

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
    color: #4f46e5
    text-decoration: none
    border-bottom: 1px solid rgba(79, 70, 229, 0.35)
    transition: color 0.15s ease, border-color 0.15s ease
  :deep(a:hover)
    color: #4338ca
    border-bottom-color: rgba(79, 70, 229, 0.85)

  :deep(table)
    display: block
    width: 100%
    max-width: 100%
    overflow-x: auto
    table-layout: auto
    margin: 0.85em 0 1em
    border-collapse: collapse
    border: 1px solid rgba(100, 116, 139, 0.32)
    border-radius: 12px
    background: rgba(255, 255, 255, 0.78)
    font-size: 13px
    line-height: 1.55
    box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04)

  :deep(thead)
    background: rgba(99, 102, 241, 0.08)

  :deep(th),
  :deep(td)
    min-width: 96px
    padding: 9px 12px
    border: 1px solid rgba(100, 116, 139, 0.22)
    text-align: left
    vertical-align: top

  :deep(th)
    color: #0f172a
    font-weight: 700
    letter-spacing: 0.01em
  :deep(td)
    color: #334155

  // ===== Code block (with header: lang label + copy button) =====
  :deep(.code-block)
    width: 100%
    max-width: 100%
    min-width: 0
    margin: 0.85em 0
    border-radius: 12px
    overflow: hidden
    background: linear-gradient(180deg, #0f172a 0%, #0b1120 100%)
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
    color: #ffffff

  :deep(.code-block__copy.is-copied)
    background: rgba(34, 197, 94, 0.18)
    border-color: rgba(34, 197, 94, 0.5)
    color: #bbf7d0

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
    color: #e5e7eb
    line-height: 1.6
    font-size: 13.5px
    overflow-x: auto
    white-space: pre

  // 兜底：万一未走 fence renderer 的 <pre>
  :deep(pre)
    width: 100%
    max-width: 100%
    min-width: 0
    overflow-x: auto
    margin: 0.8em 0
    padding: 14px 16px
    border-radius: 12px
    background: linear-gradient(180deg, #0f172a 0%, #0b1120 100%)
    color: #e5e7eb
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
    border-left: 3px solid rgba(99, 102, 241, 0.55)
    background: rgba(99, 102, 241, 0.06)
    border-radius: 0 8px 8px 0
    color: rgba(31, 41, 55, 0.82)

  :deep(hr)
    margin: 1.1em 0
    border: 0
    border-top: 1px solid rgba(100, 116, 139, 0.22)

  :deep(img)
    max-width: 100%
    border-radius: 10px

// ===== Sent (user) variant — text on gradient bubble =====
.chat-message-content--sent
  text-align: right

  :deep(h1),
  :deep(h2),
  :deep(h3),
  :deep(h4),
  :deep(h5),
  :deep(h6),
  :deep(p),
  :deep(li)
    color: #ffffff

  :deep(a)
    color: #ffffff
    border-bottom-color: rgba(255, 255, 255, 0.55)
  :deep(a:hover)
    border-bottom-color: #ffffff

  :deep(code)
    background: rgba(255, 255, 255, 0.20)
    color: #ffffff

  :deep(.code-block)
    background: rgba(15, 23, 42, 0.62)
    box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.16)

  :deep(.code-block__header)
    background: rgba(15, 23, 42, 0.55)
    border-bottom-color: rgba(255, 255, 255, 0.18)

  :deep(.code-block__copy)
    border-color: rgba(255, 255, 255, 0.32)
    background: rgba(255, 255, 255, 0.10)
    color: #ffffff
  :deep(.code-block__copy:hover)
    background: rgba(255, 255, 255, 0.20)
    border-color: rgba(255, 255, 255, 0.6)

  :deep(blockquote)
    background: rgba(255, 255, 255, 0.10)
    border-left-color: rgba(255, 255, 255, 0.7)
    color: rgba(255, 255, 255, 0.95)

  :deep(table)
    background: rgba(255, 255, 255, 0.10)
    border-color: rgba(255, 255, 255, 0.30)
  :deep(thead)
    background: rgba(255, 255, 255, 0.18)
  :deep(th),
  :deep(td)
    color: #ffffff
    border-color: rgba(255, 255, 255, 0.24)

// ===== Dark theme content =====
.chat-message-content--dark
  color: #f1f5f9

  :deep(h1),
  :deep(h2),
  :deep(h3),
  :deep(h4),
  :deep(h5),
  :deep(h6)
    color: #f8fafc

  :deep(p),
  :deep(li),
  :deep(span),
  :deep(strong),
  :deep(em),
  :deep(del)
    color: inherit

  :deep(a)
    color: #93c5fd
    border-bottom-color: rgba(147, 197, 253, 0.45)
  :deep(a:hover)
    color: #bfdbfe
    border-bottom-color: rgba(191, 219, 254, 0.85)

  :deep(table)
    background: rgba(15, 23, 42, 0.55)
    border-color: rgba(203, 213, 225, 0.22)
  :deep(thead)
    background: rgba(99, 102, 241, 0.22)
  :deep(th)
    color: #f8fafc
  :deep(td)
    color: #e2e8f0
  :deep(th),
  :deep(td)
    border-color: rgba(203, 213, 225, 0.18)

  :deep(code)
    background: rgba(226, 232, 240, 0.14)
    color: #e2e8f0

  :deep(blockquote)
    background: rgba(99, 102, 241, 0.12)
    color: rgba(248, 250, 252, 0.85)
    border-left-color: rgba(147, 197, 253, 0.7)

  :deep(hr)
    border-top-color: rgba(203, 213, 225, 0.18)

// ===== Responsive =====
@media (max-width: 1280px)
  .chat-messages
    padding-left: 56px
    padding-right: 56px

  .chat-q-message
    :deep(.q-message-received .q-message-container)
      padding-right: 96px
    :deep(.q-message-sent .q-message-container)
      padding-left: 96px
    :deep(.q-message-sent .col-grow)
      max-width: min(640px, 100%)
    :deep(.q-message-received .col-grow)
      max-width: min(900px, 100%)

@media (max-width: 900px)
  .chat-messages
    padding-left: 28px
    padding-right: 28px

  .chat-q-message
    :deep(.q-message-received .q-message-container)
      padding-right: 56px
    :deep(.q-message-sent .q-message-container)
      padding-left: 56px
    :deep(.q-message-sent .col-grow)
      max-width: min(520px, 100%)
    :deep(.q-message-received .col-grow)
      max-width: min(720px, 100%)

  .chat-message-bubble
    padding: 11px 14px

@media (max-width: 599px)
  .chat-messages
    padding-left: 14px
    padding-right: 14px
    padding-top: 18px
    padding-bottom: 24px

  .chat-q-message
    & + .chat-q-message
      margin-top: 22px
    & + .chat-q-message--continued
      margin-top: 4px
    :deep(.q-message-received .q-message-container),
    :deep(.q-message-sent .q-message-container)
      padding-left: 0
      padding-right: 0
      column-gap: 10px
    :deep(.col-grow)
      max-width: 100%

  .chat-message-bubble
    padding: 10px 12px
    font-size: 14.5px
    line-height: 1.7

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
  .chat-q-message--streaming .chat-message-bubble::before
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
  color: #92400e
  margin-bottom: 6px
  text-transform: uppercase
  letter-spacing: 0.04em

:global(.body--dark) .chat-pending-label
  color: #fbbf24

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
  color: #1e293b
  line-height: 1.4

:global(.body--dark) .chat-pending-item__content
  color: #e2e8f0

.chat-pending-item__meta
  display: flex
  gap: 8px
  margin-top: 2px
  font-size: 11px
  color: #64748b

:global(.body--dark) .chat-pending-item__meta
  color: #94a3b8

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
