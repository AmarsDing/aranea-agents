// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <q-card flat bordered class="memory-card">
    <q-card-section>
      <div class="text-h6">{{ t('memory.settingsStatus.title') }}</div>
      <div class="text-caption text-grey-7">
        {{ t('memory.settingsStatus.subtitle') }}
      </div>
    </q-card-section>
    <q-list separator>
      <q-item v-for="item in items" :key="item.label">
        <q-item-section avatar>
          <q-icon
            :name="item.done ? 'check_circle' : 'radio_button_unchecked'"
            :color="item.done ? 'positive' : 'grey'"
          />
        </q-item-section>
        <q-item-section>
          <q-item-label>{{ item.label }}</q-item-label>
          <q-item-label caption>{{ item.caption }}</q-item-label>
        </q-item-section>
      </q-item>
    </q-list>
  </q-card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';

const { t } = useI18n();

defineProps<{
  items: Array<{
    label: string;
    caption: string;
    done: boolean;
  }>;
}>();
</script>
