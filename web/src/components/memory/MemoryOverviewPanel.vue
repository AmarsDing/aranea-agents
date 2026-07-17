// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <div class="row q-col-gutter-md">
    <div class="col-12 col-lg-7">
      <q-card flat bordered class="memory-card">
        <q-card-section>
          <div class="text-h6">{{ t('memory.overview.flowTitle') }}</div>
          <div class="text-caption text-grey-7">
            {{ t('memory.overview.flowCaption') }}
          </div>
        </q-card-section>
        <q-card-section>
          <div class="memory-flow">
            <div v-for="layer in memoryLayers" :key="layer.key" class="memory-flow-node">
              <q-avatar :color="layer.color" text-color="white" :icon="layer.icon" />
              <div class="col">
                <div class="text-subtitle2 text-weight-bold">{{ layer.title }}</div>
                <div class="text-caption text-grey-7">{{ layer.caption }}</div>
              </div>
              <q-chip dense square :color="layer.statusColor" text-color="white">{{ layer.status }}</q-chip>
            </div>
          </div>
        </q-card-section>
      </q-card>
    </div>
    <div class="col-12 col-lg-5">
      <q-card flat bordered class="memory-card">
        <q-card-section>
          <div class="text-h6">{{ t('memory.overview.actionTitle') }}</div>
          <div class="text-caption text-grey-7">{{ t('memory.overview.actionCaption') }}</div>
        </q-card-section>
        <q-list separator>
          <q-item v-for="item in actionItems" :key="item.title">
            <q-item-section avatar>
              <q-avatar :color="item.color" text-color="white" :icon="item.icon" />
            </q-item-section>
            <q-item-section>
              <q-item-label>{{ item.title }}</q-item-label>
              <q-item-label caption>{{ item.caption }}</q-item-label>
            </q-item-section>
            <q-item-section side>
              <q-chip dense :color="item.color" text-color="white">{{ item.count }}</q-chip>
            </q-item-section>
          </q-item>
        </q-list>
      </q-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';

const { t } = useI18n();

defineProps<{
  memoryLayers: Array<{
    key: string;
    title: string;
    caption: string;
    icon: string;
    color: string;
    status: string;
    statusColor: string;
  }>;
  actionItems: Array<{
    title: string;
    caption: string;
    count: number;
    icon: string;
    color: string;
  }>;
}>();
</script>
