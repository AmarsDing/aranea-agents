<template>
  <q-card flat bordered class="settings-section q-mt-md">
    <q-card-section>
      <div class="text-subtitle2">{{ t("agentSettings.channelRefsTitle") }}</div>
      <div class="text-caption text-grey-7 q-mb-sm">{{ t("agentSettings.channelRefsHint") }}</div>

      <div v-if="loading" class="text-center q-pa-md">
        <q-spinner-dots size="28px" color="primary" />
      </div>

      <q-banner v-else-if="loadError" rounded class="bg-negative text-white">
        {{ loadError }}
        <template #action>
          <q-btn flat color="white" :label="t('agentSettings.retry')" @click="reload" />
        </template>
      </q-banner>

      <div v-else-if="refs.length === 0" class="text-grey-7 q-pa-sm">
        {{ t("agentSettings.noChannelRefs") }}
      </div>

      <q-list v-else dense separator>
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
import { useI18n } from "vue-i18n";
import { useAgentChannelRefs } from "../../features/agents/useAgentChannelRefs";

const props = defineProps<{
  agentId: string;
  agentKey: string;
}>();

const { t } = useI18n();

const { refs, loading, loadError, channelTypeLabel, openChannels, reload } = useAgentChannelRefs(
  () => props.agentId,
  () => props.agentKey
);
</script>
