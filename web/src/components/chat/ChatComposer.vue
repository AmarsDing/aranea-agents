<template>
  <q-card-section class="chat-composer q-pa-sm q-pa-md-sm">
    <q-banner
      v-if="isAwaitingUser && awaitKind === AWAIT_KIND_TOOL_CONFIRM"
      rounded
      class="q-mb-sm app-banner-warning"
      dense
    >
      <template #avatar>
        <q-icon name="gpp_maybe" color="warning" />
      </template>
      <div class="text-body2">{{ t("chat.toolConfirmHint") }}</div>
      <div v-if="awaitToolKey" class="text-caption q-mt-xs">
        {{ t("chat.toolConfirmTool") }}: <code>{{ awaitToolKey }}</code>
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
        @enqueue="(text) => $emit('enqueue-message', text)"
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
          class="chat-toolbar-field"
          :dark="isDark"
          @update:model-value="$emit('update:selectedKnowledgeBases', ($event as string[]) ?? [])"
        />
        <div class="chat-toolbar-actions row items-center no-wrap q-gutter-sm">
          <div class="chat-context-pill row items-center no-wrap">
            <span class="text-caption text-no-wrap q-mr-sm">{{ t("chat.contextPromptUse") }}</span>
            <q-circular-progress
              :value="contextRatio * 100"
              show-value
              size="34px"
              :thickness="0.2"
              color="primary"
            >
              <span class="text-caption">{{ Math.round(contextRatio * 100) }}%</span>
            </q-circular-progress>
            <span v-if="sessionTotalTokens != null" class="text-caption text-no-wrap q-ml-sm sessions-muted">
              {{ t("chat.sessionTotalTokens", { n: sessionTotalTokens }) }}
            </span>
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
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import ChatEnqueueMessage from "../../features/chat/components/ChatEnqueueMessage.vue";
import { AWAIT_KIND_TOOL_CONFIRM } from "../../features/chat/awaitConstants";
import type { ChatAttachment } from "./types";

type Option = { label: string; value: string; caption?: string };

defineProps<{
  modelValue: string;
  attachments: ChatAttachment[];
  dialogMode: string;
  modelProvider: string;
  modeOptions: Option[];
  providerOptions: Option[];
  contextRatio: number;
  sessionTotalTokens?: number | null;
  knowledgeBaseOptions?: Option[];
  selectedKnowledgeBases?: string[];
  isDark: boolean;
  sending?: boolean;
  inputDisabled?: boolean;
  isAwaitingUser?: boolean;
  awaitKind?: string;
  awaitToolKey?: string;
  showEnqueue?: boolean;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: string];
  "update:dialogMode": [value: string];
  "update:modelProvider": [value: string];
  "update:selectedKnowledgeBases": [value: string[]];
  "remove-attachment": [id: string];
  "pick-file": [];
  voice: [];
  send: [];
  stop: [];
  "enqueue-message": [content: string];
  "submit-await-reply": [];
  "submit-tool-confirm": [approved: boolean];
}>();

const { t } = useI18n();

function onInputKeydown(event: KeyboardEvent) {
  if (event.key !== "Enter" || event.shiftKey || event.isComposing) return;
  event.preventDefault();
  emit("send");
}
</script>
