<script setup lang="ts">
withDefaults(defineProps<{ kind?: 'radar' | 'candle'; tone?: string }>(), { kind: 'radar', tone: 'ai' });
</script>

<template>
  <svg v-if="kind === 'radar'" class="ec-radar" viewBox="0 0 64 64" aria-hidden="true">
    <circle cx="32" cy="32" r="29" />
    <circle cx="32" cy="32" r="21" />
    <circle cx="32" cy="32" r="12" />
    <g class="ec-radar__sweep">
      <path d="M32 32 L13 10 A29 29 0 0 1 57 48Z" />
      <path d="M32 32 L13 10" />
    </g>
    <circle cx="32" cy="32" r="2" />
  </svg>
  <svg v-else class="ec-candle" :class="`ec-tone-${tone}`" viewBox="0 0 64 84" aria-hidden="true">
    <path class="ec-candle__flame" d="M32 3 C15 23 28 28 32 28 C43 25 41 17 32 3Z" />
    <rect x="13" y="34" width="39" height="47" rx="7" class="ec-candle__jar" />
    <rect x="13" y="45" width="39" height="19" class="ec-candle__label" />
    <path d="M15 35 H50" class="ec-candle__rim" />
  </svg>
</template>
