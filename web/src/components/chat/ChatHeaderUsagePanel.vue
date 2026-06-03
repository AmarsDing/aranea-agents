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
      {{ t('chat.contextUsageEmpty', '暂无用量数据') }}
    </span>
    <q-btn
      v-if="showCompactBtn"
      flat
      round
      dense
      icon="o_compress"
      size="sm"
      :loading="compactLoading"
      :disable="compactLoading"
      class="chat-header-usage__compact-btn"
      :class="{ 'chat-header-usage__compact-btn--dark': isDark }"
      @click="onCompactClick"
    >
      <q-tooltip :delay="400">{{ t('chat.compactSession', '压缩上下文') }}</q-tooltip>
    </q-btn>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import {
  composerContextColor,
  composerUsageParts,
  type ComposerUsageSnapshot,
} from '../../features/chat/composerUsageMetrics';
import type { PromptBreakdown } from '../../features/chat/contextBreakdown';
import ChatContextBreakdownPopover from './ChatContextBreakdownPopover.vue';

const props = withDefaults(
  defineProps<{
    contextRatio: number;
    contextStatus?: string;
    usageSnapshot?: ComposerUsageSnapshot | null;
    isDark?: boolean;
    breakdown?: PromptBreakdown | null;
    sessionId?: string | null;
  }>(),
  { isDark: false },
);

const emit = defineEmits<{
  compact: [sessionId: string];
}>();

const { t } = useI18n();
const $q = useQuasar();

const popoverOpen = ref(false);
const compactLoading = ref(false);

const clampedRatio = computed(() => Math.min(1, Math.max(0, props.contextRatio ?? 0)));
const showCompactBtn = computed(() => props.sessionId && clampedRatio.value >= 0.4);
const pctLabel = computed(() => `${Math.round(clampedRatio.value * 100)}%`);
const ringColor = computed(() => {
  const status = props.contextStatus?.trim();
  if (status) return composerContextColor(status);
  if (clampedRatio.value >= 0.8) return composerContextColor(undefined, clampedRatio.value);
  return 'accent';
});

const usageParts = computed(() => (props.usageSnapshot ? composerUsageParts(props.usageSnapshot) : []));

const hasBreakdown = computed(() => props.breakdown != null && props.breakdown.categories.length > 0);

const isPrecise = computed(
  () =>
    props.usageSnapshot?.promptBreakdown != null &&
    Object.values(props.usageSnapshot.promptBreakdown).some((v) => v != null && v > 0),
);

function onRingClick() {
  if (hasBreakdown.value) {
    popoverOpen.value = !popoverOpen.value;
  }
}

async function onCompactClick() {
  if (!props.sessionId || compactLoading.value) return;
  compactLoading.value = true;
  try {
    emit('compact', props.sessionId);
  } finally {
    setTimeout(() => {
      compactLoading.value = false;
    }, 2000);
  }
}

const ariaLabel = computed(() => {
  const detail = usageParts.value.join(' · ');
  return detail
    ? `${t('chat.contextPromptUse')} ${pctLabel.value} · ${detail}`
    : `${t('chat.contextPromptUse')} ${pctLabel.value}`;
});
</script>

<style lang="sass">
.chat-header-usage__compact-btn
  margin-left: 4px
  opacity: 0.5
  transition: opacity 0.2s
  &:hover
    opacity: 1
  &--dark
    color: var(--color-accent)
</style>
