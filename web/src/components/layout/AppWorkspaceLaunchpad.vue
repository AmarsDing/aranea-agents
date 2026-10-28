<template>
  <nav class="ui-bento-grid" :aria-label="label">
    <RouterLink
      v-for="item in items"
      :key="item.to"
      :to="item.to"
      class="ui-bento-card"
      :class="[`ui-bento-card--${item.tone ?? 'cyan'}`, { 'ui-bento-card--hero': item.featured }]"
    >
      <span class="ui-card-icon" aria-hidden="true"><q-icon :name="item.icon" /></span>
      <div class="ui-card-content">
        <h2 class="ui-card-title">{{ item.title }}</h2>
        <p class="ui-card-description">{{ item.description }}</p>
      </div>
      <span v-if="item.featured" class="ui-card-cta"
        >{{ item.action }} <q-icon name="arrow_forward" aria-hidden="true"
      /></span>
      <q-icon v-else name="north_east" class="ui-card-arrow" aria-hidden="true" />
    </RouterLink>
  </nav>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router';

defineProps<{
  label: string;
  items: ReadonlyArray<{
    to: string;
    title: string;
    description: string;
    icon: string;
    tone?: string;
    featured?: boolean;
    action?: string;
  }>;
}>();
</script>
