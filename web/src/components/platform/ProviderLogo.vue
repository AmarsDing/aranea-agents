<template>
  <q-avatar :size="size" class="provider-logo" :class="{ 'provider-logo--loaded': !!svg }">
    <span v-if="svg" class="provider-logo__svg" v-html="svg" />
    <q-icon v-else :name="fallbackIcon" />
  </q-avatar>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue';
import { fetchProviderLogoSvg } from '../../features/model-catalog/providerLogo';

const props = withDefaults(
  defineProps<{
    providerId: string;
    size?: string;
    fallbackIcon?: string;
  }>(),
  {
    size: '32px',
    fallbackIcon: 'memory',
  },
);

const svg = ref('');

async function load() {
  const id = props.providerId.trim();
  if (!id) {
    svg.value = '';
    return;
  }
  svg.value = await fetchProviderLogoSvg(id);
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
