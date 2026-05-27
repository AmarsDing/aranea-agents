<template>
  <div v-if="sessionId" class="chat-composer-toolbar-slot">
    <q-btn
      dense
      unelevated
      outline
      color="primary"
      class="chat-toolbar-btn chat-toolbar-btn--outline"
      :aria-label="triggerLabel"
      @click="dialogOpen = true"
    >
      <q-icon name="inventory_2" size="20px" />
      <q-badge v-if="items.length" color="primary" floating transparent>{{ items.length }}</q-badge>
      <q-tooltip>{{ triggerLabel }}</q-tooltip>
    </q-btn>

    <q-dialog v-model="dialogOpen" transition-show="scale" transition-hide="scale">
      <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog" :class="{ 'is-dark': isDark }">
        <q-card-section class="row items-center q-pb-none">
          <div class="text-h6 col">{{ t("chat.sessionArtifacts.title", "会话制品") }}</div>
          <q-btn flat round dense icon="close" v-close-popup :aria-label="t('chat.cancel')" />
        </q-card-section>
        <q-card-section class="app-dialog-body q-pt-sm">
          <ArtifactList :items="items" :loading="loading" @open="onOpen" @deleted="(id) => emit('deleted', id)" />
        </q-card-section>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import ArtifactList from "../../features/artifact/ArtifactList.vue";
import type { ArtifactMeta } from "../../features/artifact/types";

const props = defineProps<{
  sessionId: string;
  items: ArtifactMeta[];
  loading?: boolean;
  isDark?: boolean;
}>();

const emit = defineEmits<{
  open: [id: string];
  deleted: [id: string];
}>();

const { t } = useI18n();
const dialogOpen = ref(false);

const triggerLabel = computed(() => {
  const base = t("chat.sessionArtifacts.title", "会话制品");
  return props.items.length ? `${base} (${props.items.length})` : base;
});

function onOpen(id: string) {
  emit("open", id);
}
</script>
