// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <div class="row q-col-gutter-md">
    <div v-for="panel in panels" :key="panel.title" class="col-12 col-md-6">
      <q-card flat bordered class="memory-card full-height">
        <q-card-section>
          <q-avatar :color="panel.color" text-color="white" :icon="panel.icon" class="q-mb-sm" />
          <div class="text-h6">{{ panel.title }}</div>
          <div class="text-body2 text-grey-7 q-mt-sm">{{ panel.caption }}</div>
        </q-card-section>
        <q-card-section>
          <q-banner rounded class="memory-info-banner">{{ panel.state }}</q-banner>
          <q-list v-if="panel.items?.length" dense class="q-mt-sm">
            <q-item v-for="item in panel.items" :key="item">
              <q-item-section>
                <q-item-label class="ellipsis">{{ item }}</q-item-label>
              </q-item-section>
            </q-item>
          </q-list>
        </q-card-section>
      </q-card>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  panels: Array<{
    title: string;
    caption: string;
    state: string;
    icon: string;
    color: string;
    items?: string[];
  }>;
}>();
</script>
