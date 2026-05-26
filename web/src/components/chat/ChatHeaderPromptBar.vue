<template>
  <div class="chat-header-prompt">
    <transition name="chat-title-fade" mode="out-in">
      <div
        :key="promptKey"
        class="chat-header-prompt__box"
        :class="{ 'chat-header-prompt__box--placeholder': isPlaceholder }"
      >
        <div class="chat-header-prompt__text ellipsis">
          {{ displayLine }}
          <q-tooltip
            v-if="showTooltip"
            anchor="bottom middle"
            self="top middle"
            :offset="[0, 8]"
            class="chat-header-prompt__tooltip"
          >
            <div class="chat-header-prompt__tooltip-body">{{ fullText }}</div>
          </q-tooltip>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";

const props = defineProps<{
  fullText: string;
  promptKey: string;
  sessionTitle?: string;
  hasMessages: boolean;
}>();

const { t } = useI18n();

const isPlaceholder = computed(() => !props.fullText.trim() && props.hasMessages);

const displayLine = computed(() => {
  const prompt = props.fullText.trim();
  if (prompt) return prompt;
  if (!props.hasMessages) {
    return (props.sessionTitle ?? "").trim() || t("chat.untitledSession");
  }
  return t("chat.headerPromptPlaceholder", "向上滚动查看该轮提问");
});

const showTooltip = computed(() => props.fullText.trim().length > 0);
</script>
