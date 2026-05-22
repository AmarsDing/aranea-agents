<template>
  <q-card v-if="refs.length" flat bordered class="settings-section q-mt-md">
    <q-card-section>
      <div class="text-subtitle2">{{ t("agentSettings.channelRefsTitle") }}</div>
      <div class="text-caption text-grey-7 q-mb-sm">{{ t("agentSettings.channelRefsHint") }}</div>
      <q-list dense separator>
        <q-item v-for="ch in refs" :key="ch.id" clickable @click="openChannels">
          <q-item-section>
            <q-item-label>{{ ch.name || ch.key }}</q-item-label>
            <q-item-label caption>{{ channelTypeLabel(ch) }} · {{ ch.key }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-icon name="chevron_right" />
          </q-item-section>
        </q-item>
      </q-list>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRouter } from "vue-router";
import { listChannels } from "../../features/channels/api";
import { channelsReferencingAgent } from "../../features/channels/channelAgentRefs";
import type { ChannelRow } from "../../features/channels/types";

const props = defineProps<{
  agentId: string;
  agentKey: string;
}>();

const { t } = useI18n();
const router = useRouter();
const all = ref<ChannelRow[]>([]);

const refs = computed(() => channelsReferencingAgent(all.value, props.agentId, props.agentKey));

onMounted(async () => {
  try {
    all.value = await listChannels();
  } catch {
    all.value = [];
  }
});

function channelTypeLabel(ch: ChannelRow): string {
  try {
    const cfg = JSON.parse(ch.config_json || "{}") as { type?: string };
    return cfg.type || "channel";
  } catch {
    return "channel";
  }
}

function openChannels() {
  void router.push({ name: "channels" });
}
</script>
