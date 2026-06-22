<template>
  <q-avatar :size="size" class="provider-logo" :class="{ 'provider-logo--loaded': !!svg }">
    <!-- eslint-disable-next-line vue/no-v-html -- SVG logo from controlled provider catalog -->
    <span v-if="svg" class="provider-logo__svg" v-html="svg" />
    <q-icon v-else :name="fallbackIcon" />
  </q-avatar>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue';

const props = withDefaults(
  defineProps<{
    providerId: string;
    size?: string;
    fallbackIcon?: string;
    fetchSvg?: (id: string) => Promise<string>;
  }>(),
  {
    size: '32px',
    fallbackIcon: 'memory',
    fetchSvg: undefined,
  },
);

const svg = ref('');

async function load() {
  const id = props.providerId.trim();
  if (!id || !props.fetchSvg) {
    svg.value = '';
    return;
  }
  svg.value = await props.fetchSvg(id);
}

onMounted(() => {
  void load();
});

watch(
  () => props.providerId,
  () => {
    void load();
  },
);
</script>
