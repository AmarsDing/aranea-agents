<template>
  <!--
    工作台 › 对话：只做聊天——问助手、问某位同事，不要求交付。
    要办成一件事（有交付、要跟进度、要验收）走「任务」：页签行右边的「转成任务」打开交办面板。
    设计：docs/aranea-agents/plan/2026-11-21-任务入口与执行统筹方案.md §3（对话页收窄为聊天）
  -->
  <ChatWorkspace />
  <Teleport defer to="#app-tab-actions">
    <q-btn
      flat
      dense
      no-caps
      icon="assignment_add"
      :label="t('cc.toTask')"
      :title="t('cc.toTaskTip')"
      @click="toTask"
    />
  </Teleport>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { useRoute } from 'vue-router';
import ChatWorkspace from '../features/chat/ChatWorkspace.vue';
import { openAssign } from '../features/conversations/assign';
import { getSession } from '../features/session/api';

const { t } = useI18n();
const route = useRoute();

// 带上这段对话的标题当任务的第一行（当前会话在地址里）。
// shortcut: 只带标题，不带对话内容的摘要——那要对话引擎暴露消息或由后端生成，放到 P3。
async function toTask() {
  const id = typeof route.query.session === 'string' ? route.query.session : '';
  const title = id
    ? await getSession(id).then(
        (x) => x.title,
        () => '',
      )
    : '';
  openAssign({ text: title });
}
</script>
