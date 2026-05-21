<template>
  <q-chat-message
    v-if="!bundle.suppressToolRow"
    class="chat-q-message"
    :class="{
      'chat-q-message--continued': row.isContinued(index),
      'chat-q-message--streaming': row.isStreaming(message),
      'chat-q-message--member': row.isTeamMember(message),
    }"
    :sent="message.role === 'user'"
    bg-color="transparent"
    text-color="inherit"
    size="grow"
  >
    <template #avatar>
      <q-avatar
        v-if="message.role === 'user'"
        :size="avatarSize"
        class="message-avatar message-avatar--user self-start"
        rounded
        icon="person"
        :aria-label="row.displayMessageName(message)"
      />
      <q-avatar
        v-else
        :size="avatarSize"
        :color="row.messageAvatarColor(message)"
        text-color="white"
        class="message-avatar self-start"
        :aria-label="row.displayMessageName(message)"
      >
        <resolved-avatar-img
          v-if="shouldRenderAgentAvatarImage(row.messageAvatarRawIcon(message))"
          :icon="row.messageAvatarRawIcon(message)"
          :alt="row.displayMessageName(message)"
        />
        <q-icon
          v-else-if="row.messageAvatarIcon(message)"
          :name="row.messageAvatarIcon(message)"
          :size="avatarIconSize"
        />
        <span v-else class="message-avatar__initials">{{ row.messageAvatarInitials(message) }}</span>
      </q-avatar>
    </template>
    <div
      class="chat-message-stack"
      :class="{ 'chat-message-stack--sent': message.role === 'user' }"
    >
      <div
        v-if="!row.isContinued(index)"
        class="message-meta-row"
        :class="{ 'message-meta-row--sent': message.role === 'user' }"
      >
        <span class="message-name">{{ row.displayMessageName(message) }}</span>
        <q-chip
          v-if="row.teamMemberMeta(message)?.role"
          dense
          size="sm"
          outline
          color="primary"
          class="q-ml-xs"
        >
          {{ row.teamMemberMeta(message)?.role }}
        </q-chip>
        <span class="message-stamp">{{ formatStamp(message.created_at) }}</span>
      </div>
      <div
        class="chat-message-bubble"
        :class="{
          'chat-message-bubble--sent': message.role === 'user',
          'chat-message-bubble--received': message.role !== 'user',
          'chat-message-bubble--dark': isDark,
          'chat-message-bubble--member': row.isTeamMember(message),
          'chat-message-bubble--tool': row.isToolEventMessage(message),
          'chat-message-bubble--tool-running': message.status === 'tool_running',
          'chat-message-bubble--tool-failed': message.status === 'tool_failed',
        }"
        :style="row.bubbleAccentStyle(message)"
      >
      <ChatExecutionCard
        v-if="bundle.structuredToolEvent"
        :event="bundle.structuredToolEvent"
        :show-member-label="isTeamSession"
      />
      <details v-else-if="row.isCollapsibleToolDetail(message)" class="chat-tool-details">
        <summary class="chat-tool-details__summary">
          <span class="chat-tool-details__summary-text">{{ row.toolCollapseSummary(message) }}</span>
          <span class="chat-tool-details__hint text-caption" aria-hidden="true" />
        </summary>
        <div
          class="chat-message-content chat-message-prose chat-tool-details__body"
          :class="{
            'chat-message-content--sent': message.role === 'user',
            'chat-message-content--dark': message.role !== 'user' && isDark,
          }"
          v-html="renderMarkdown(row.toolCollapseDetail(message))"
        />
      </details>
      <template v-else>
        <details v-if="bundle.presentation.reasoning" class="chat-reasoning-details q-mb-sm">
          <summary class="text-caption text-weight-medium">{{ row.t("chat.reasoningTitle", "思考过程") }}</summary>
          <div
            class="chat-message-content chat-message-prose chat-reasoning-details__body"
            :class="{ 'chat-message-content--dark': isDark }"
            v-html="renderMarkdown(bundle.presentation.reasoning)"
          />
        </details>
        <ChatReactSteps
          v-if="bundle.reactStepsWithTools.length"
          :steps="bundle.reactStepsWithTools"
          :is-dark="isDark"
        />
        <ChatA2UIPreview
          v-if="bundle.presentation.mode === 'a2ui' && bundle.presentation.a2uiLines"
          :lines="bundle.presentation.a2uiLines"
          @user-action="(p) => emit('a2ui-user-action', p)"
        />
        <div
          v-if="bundle.presentation.bodyMarkdown"
          class="chat-message-content chat-message-prose"
          :class="{
            'chat-message-content--sent': message.role === 'user',
            'chat-message-content--dark': message.role !== 'user' && isDark,
          }"
          v-html="
            row.isStreaming(message)
              ? renderStreamingMarkdown(bundle.presentation.bodyMarkdown)
              : renderMarkdown(bundle.presentation.bodyMarkdown)
          "
        />
      </template>
      <div
        v-if="message.role !== 'user' && message.status === 'error' && row.assistantErrorDetail(message)"
        class="text-caption text-negative q-mt-xs chat-assistant-error"
      >
        {{ row.assistantErrorDetail(message) }}
      </div>
      <div v-if="message.role === 'user'" class="message-send-tags message-send-tags--sent text-caption">
        {{ row.userSendTagLine(message) }}
      </div>
      <span v-if="row.isStreaming(message)" class="chat-typing" aria-label="正在输入">
        <i /><i /><i />
      </span>
      </div>
    </div>
  </q-chat-message>
</template>

<script setup lang="ts">
import { computed, toRef } from "vue";
import ResolvedAvatarImg from "../avatar/ResolvedAvatarImg.vue";
import ChatExecutionCard from "./ChatExecutionCard.vue";
import ChatReactSteps from "./ChatReactSteps.vue";
import ChatA2UIPreview from "./ChatA2UIPreview.vue";
import { buildMessagePresentation } from "../../features/chat/messagePlannerPresentation";
import type { Message, ReactToolLinkIndex } from "../../features/chat/types";
import { shouldRenderAgentAvatarImage } from "../../features/avatar/iconModel";
import {
  formatMessageStamp,
  renderChatMarkdown,
  renderStreamingChatMarkdown,
} from "../../features/chat/chatMessageMarkdown";
import {
  CHAT_MESSAGE_AVATAR_ICON_SIZE,
  CHAT_MESSAGE_AVATAR_SIZE,
  useChatMessageRow,
} from "../../features/chat/useChatMessageRow";
import type { A2UIUserActionPayload } from "../../features/chat/a2uiUserAction";

const emit = defineEmits<{
  "a2ui-user-action": [payload: A2UIUserActionPayload];
}>();

const props = defineProps<{
  message: Message;
  index: number;
  messages: Message[];
  isDark: boolean;
  isTeamSession?: boolean;
  /** Active agent planner_kind for presentation (react / a2ui). */
  plannerKind?: string;
  reactToolLinkIndex: ReactToolLinkIndex;
}>();

const messagesRef = computed(() => props.messages);
const row = useChatMessageRow(messagesRef);

const bundle = computed(() =>
  buildMessagePresentation(
    props.plannerKind ?? "",
    props.message,
    props.index,
    props.reactToolLinkIndex
  )
);
const avatarSize = CHAT_MESSAGE_AVATAR_SIZE;
const avatarIconSize = CHAT_MESSAGE_AVATAR_ICON_SIZE;

function formatStamp(iso: string) {
  return formatMessageStamp(iso);
}

function renderMarkdown(content: string) {
  return renderChatMarkdown(content);
}

function renderStreamingMarkdown(content: string) {
  return renderStreamingChatMarkdown(content);
}
</script>
