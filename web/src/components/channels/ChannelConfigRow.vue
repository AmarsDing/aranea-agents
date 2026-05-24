<template>
  <div class="app-musebot-row">
    <div class="app-musebot-row__label-wrap">
      <label class="app-musebot-row__label" :for="inputId">{{ label }}</label>
      <q-icon
        v-if="help"
        name="help_outline"
        size="14px"
        class="app-musebot-row__help"
        :aria-label="helpAriaLabel"
      >
        <q-tooltip max-width="340px" anchor="top middle" self="bottom middle">
          <div class="app-field-help-tooltip">
            <div>{{ help.description }}</div>
            <div v-if="help.example" class="app-field-help-tooltip__example">
              {{ helpExamplePrefix }}{{ help.example }}
            </div>
          </div>
        </q-tooltip>
      </q-icon>
    </div>
    <div class="app-musebot-row__control">
      <slot />
    </div>
    <span v-if="status" class="app-musebot-row__status">{{ status }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { ChannelFieldHelp } from "../../features/channels/channelPlatformFields";

export type { ChannelFieldHelp };

const props = defineProps<{
  label: string;
  status?: string;
  fieldKey?: string;
  help?: ChannelFieldHelp;
}>();

const { t } = useI18n();

const inputId = computed(() => (props.fieldKey ? `channel-field-${props.fieldKey}` : undefined));
const helpAriaLabel = computed(() => t("channelEditor.fieldHelpAria"));
const helpExamplePrefix = computed(() => t("channelEditor.fieldHelpExamplePrefix"));
</script>
