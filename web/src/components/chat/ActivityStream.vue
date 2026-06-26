<template>
  <div class="event-stream">
    <template v-for="(item, idx) in renderItems" :key="item.activity.id">
      <!-- Phase transition indicator between thinking and reply -->
      <div
        v-if="idx > 0 && item.event?.kind === 'reply' && renderItems[idx - 1].event?.kind === 'thinking'"
        class="event-stream__transition"
      >
        <span class="event-stream__transition-line" />
        <span class="event-stream__transition-label">{{ t('chat.transition.thinkingToReply') }}</span>
        <span class="event-stream__transition-line" />
      </div>

      <!-- task (non-failed) → UserMessageBubble -->
      <UserMessageBubble
        v-if="item.event === null"
        :message="taskToMessage(item.activity)"
        @confirm="(id, approved) => $emit('confirm', id, approved)"
        @error-retry="(e) => $emit('error-retry', e)"
        @error-switch-model="(e) => $emit('error-switch-model', e)"
        @error-rephrase="(e)