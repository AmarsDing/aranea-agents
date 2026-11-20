<template>
  <!-- 连接领域的空态：说清这是干什么的、怎么接，给第一步动作；slot 放可直接点选的起点（平台、模板） -->
  <section class="cx-guide" :aria-label="title">
    <div class="cx-guide__head">
      <span class="cx-guide__icon"><q-icon :name="icon" size="22px" aria-hidden="true" /></span>
      <div class="min-width-0">
        <h2 class="cx-guide__title">{{ title }}</h2>
        <p class="cx-guide__desc">{{ description }}</p>
      </div>
    </div>
    <ol v-if="steps.length" class="cx-guide__steps">
      <li v-for="(step, index) in steps" :key="step">
        <span class="cx-guide__num">{{ index + 1 }}</span>
        <span>{{ step }}</span>
      </li>
    </ol>
    <slot />
    <div v-if="actionLabel || $slots.actions" class="cx-guide__actions">
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
