<template>
  <!--
    运行与治理各页的空态 / 未接入引导：说清这是干什么的、为什么现在是空的，给出步骤和第一步动作。
    shortcut: 与「连接」领域的 ConnectGuide 同形；那边还在改、未提交，两者都落库后合并成一个公共组件。
  -->
  <section class="op-guide" :aria-label="title">
    <div class="op-guide__head">
      <span class="op-guide__icon"><q-icon :name="icon" size="22px" aria-hidden="true" /></span>
      <div class="min-width-0">
        <h2 class="op-guide__title">{{ title }}</h2>
        <p class="op-guide__desc">{{ description }}</p>
      </div>
    </div>
    <ol v-if="steps.length" class="op-guide__steps">
      <li v-for="(step, index) in steps" :key="step">
        <span class="op-guide__num">{{ index + 1 }}</span>
        <span>{{ step }}</span>
      </li>
    </ol>
    <slot />
    <div v-if="actionLabel || $slots.actions" class="op-guide__actions">
      <q-btn
        v-if="actionLabel"
        unelevated
        no-caps
        color="primary"
        icon="add"
        :label="actionLabel"
        @click="$emit('action')"
      />
      <slot name="actions" />
    </div>
  </section>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{ icon: string; title: string; description: string; steps?: string[]; actionLabel?: string }>(),
  { steps: () => [], actionLabel: '' },
);
defineEmits<{ action: [] }>();
</script>
