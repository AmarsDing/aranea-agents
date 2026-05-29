<template>
  <div class="chat-header-usage row items-center no-wrap" :aria-label="ariaLabel">
    <div class="chat-header-usage__ring-wrap">
      <q-circular-progress
        :value="clampedRatio * 100"
        size="40px"
        :thickness="0.3"
        :color="ringColor"
        class="chat-header-usage__ring"
        :class="{
          'chat-header-usage__ring--dark': isDark,
          'chat-header-usage__ring--clickable': hasBreakdown,
        }"
        @click="onRingClick"
      >
        <div class="chat-header-usage__ring-value">{{ pctLabel }}</div>
      </q-circular-progress>
      <q-menu
        v-if="hasBreakdown"
        v-model="popoverOpen"
        anchor="bottom left"
        self="top left"
        :offset="[0, 8]"
        transition-show="jump-down"
        transition-hide="jump-up"
        class="ctx-breakdown-menu"
      >
        <ChatContextBreakdownPopover
          v-if="breakdown"
          :breakdown="breakdown"
          :context-status="contextStatus"
          :total-cost-micro-usd="usageSnapshot?.totalCostMicroUsd"
          :is-precise="isPrecise"
          :is-dark="isDark"
        />
      </q-menu>
    </div>
    <div v-if="usageParts.length" class="chat-header-usage__metrics row items-center no-wrap">
      <span v-for="(part, idx) in usageParts" :key="idx" class="chat-header-usage__chip">{{ part }}</span>
    </div>
    <span v-else class="chat-header-usage__chip chat-header-usage__chip--muted">
      {{ t("chat.contextUsageEmpty", "暂无用量数据") }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import {
  composerContextColor,
  composerUsageParts,
  type ComposerUsageSnapshot,
} from "../../features/chat/composerUsageMetrics";
import type { PromptBreakdown } from "../../features/chat/contextBreakdown";
import ChatContextBreakdownPopover from "./ChatContextBreakdownPopover.vue";

const props = withDefaults(
  defineProps<{
    contextRatio: number;
    contextStatus?: string;
    usageSnapshot?: ComposerUsageSnapshot | null;
    isDark?: boolean;
    breakdown?: PromptBreakdown | null;
  }>(),
  { isDark: false },
);

const { t } = useI18n();

const popoverOpen = ref(false);

const clampedRatio = computed(() => Math.min(1, Math.max(0, props.contextRatio ?? 0)));
const pctLabel = computed(() => `${Math.round(clampedRatio.value * 100)}%`);
const ringColor = computed(() => {
  const status = props.contextStatus?.trim();
  if (status) return composerContextColor(status);
  if (clampedRatio.value >= 0.8) return composerContextColor(undefined, clampedRatio.value);
  return "accent";
});

const usageParts = computed(() =>
  props.usageSnapshot ? composerUsageParts(props.usageSnapshot) : [],
);

const hasBreakdown = computed(() =>
  props.breakdown != null && props.breakdown.categories.length > 0,
);

const isPrecise = computed(() =>
  props.usageSnapshot?.promptBreakdown != null &&
  Object.values(props.usageSnapshot.promptBreakdown).some((v) => v != null && v > 0),
);

function onRingClick() {
  if (hasBreakdown.value) {
    popoverOpen.value = !popoverOpen.value;
  }
}

const ariaLabel = computed(() => {
  const detail = usageParts.value.join(" · ");
  return detail
    ? `${t("chat.contextPromptUse")} ${pctLabel.value} · ${detail}`
    : `${t("chat.contextPromptUse")} ${pctLabel.value}`;
});
</script>
