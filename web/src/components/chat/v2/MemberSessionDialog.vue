<!-- web/src/components/chat/v2/MemberSessionDialog.vue
  成员对话内容弹框（方案A）：Graph 富卡片成员行点击后弹出。
  - 替代原 TeamRunCard 折叠展开的成员面板展示方式
  - 内容复用 MemberSessionPanel（embedded 模式：始终展开、无折叠开关）
  - 纯展示组件：open 由父组件 v-model 控制，操作事件原样透传
-->
<template>
  <!-- 移动端（<600px）最大化弹框：注入输入栏成为底部操作区（72 §3.2 触控化） -->
  <q-dialog
    :model-value="open"
    :maximized="isMobileScreen"
    @update:model-value="(v: boolean) => $emit('update:open', v)"
  >
    <!-- 2026-07-27：md(720px) → lg(880px) + body 60vh→70vh，给活动流/任务指令/输入栏更充裕空间 -->
    <q-card class="app-dialog-card app-dialog-card--lg member-session-dialog">
      <q-card-section class="member-session-dialog__header">
        <span class="member-session-dialog__title">{{ title }}</span>
        <q-btn
          flat
          dense
          round
          icon="close"
          class="member-session-dialog__close"
          :aria-label="t('chat.v2.cancel')"
          @click="$emit('update:open', false)"
        />
      </q-card-section>
      <q-card-section v-if="memberSession" class="member-session-dialog__body" @click="handleCodeBlockClick">
        <MemberSessionPanel
          :member-session="memberSession"
          embedded
          @pause-agent="(sid) => $emit('pause-agent', sid)"
          @inject-agent="(p) => $emit('inject-agent', p)"
          @expand="(ids) => $emit('expand', ids)"
          @confirm-step="(p) => $emit('confirm-step', p)"
        />
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import type { MemberSession } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload } from '../../../features/chat/types';
import { useChatCodeCopy } from '../../../features/chat/composables/useChatMessageScroll';
import MemberSessionPanel from './MemberSessionPanel.vue';

function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

// Safe Quasar wrapper（同 TaskCard）：useQuasar() 是裸 inject()，无插件时返回 undefined，
// screen 回退桌面（lt.sm = false），单测中保持桌面行为。
function useSafeQuasar() {
  try {
    const $q = useQuasar();
    if ($q) return $q;
  } catch {
    // fall through to no-op
  }
  return {
    notify: (_: unknown) => {},
    screen: { lt: { sm: false } },
  } as unknown as ReturnType<typeof useQuasar>;
}

const props = defineProps<{
  open: boolean;
  memberSession: MemberSession | null;
}>();

defineEmits<{
  'update:open': [open: boolean];
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  expand: [sessionIds: string[]];
  'confirm-step': [payload: ConfirmStepPayload];
}>();

const { t } = useSafeI18n();
const $q = useSafeQuasar();

// 移动端断点：<600px 弹框最大化（与路由断点 $q.screen.lt.sm 一致）
const isMobileScreen = computed(() => $q.screen.lt.sm);

// 弹框 teleport 到 body，聊天容器的代码块复制/折叠事件委托覆盖不到——
// 在弹框 body 上复用同一处理逻辑（handleMessagesClick 内部按 closest('.code-block*') 匹配，非代码块点击自然落空）。
const { handleMessagesClick: handleCodeBlockClick } = useChatCodeCopy();

const title = computed(() => props.memberSession?.AgentName || props.memberSession?.AgentKey || '');
</script>

<style lang="sass" scoped>
.member-session-dialog
  &__header
    display: flex
    align-items: center
    gap: 8px
    padding-bottom: 4px

  &__title
    flex: 1 1 auto
    min-width: 0
    font-size: 14px
    font-weight: 600
    color: var(--color-text-primary)
    white-space: nowrap
    overflow: hidden
    text-overflow: ellipsis

  &__close
    flex: 0 0 auto

  &__body
    padding-top: 4px
    max-height: 70vh
    overflow-y: auto

// 移动端（<600px，与 $q.screen.lt.sm 一致）：最大化弹框 → 卡片 flex 列，
// body 撑满剩余高度且不整体滚动（活动流在 MemberSessionPanel 内部滚动，输入栏固定底部）。
@media (max-width: 599px)
  .member-session-dialog
    display: flex
    flex-direction: column
    height: 100%

    &__header
      flex: 0 0 auto

    &__body
      flex: 1 1 auto
      min-height: 0
      max-height: none
      overflow: hidden
      display: flex
      flex-direction: column
</style>
