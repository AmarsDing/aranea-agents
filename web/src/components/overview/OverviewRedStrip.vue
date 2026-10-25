<template>
  <section class="ov-red">
    <div class="ov-red__group">
      <header class="ov-section-head">
        <q-icon name="model_training" size="18px" class="ov-section-head__icon" />
        <h2 class="ov-section-head__title">{{ t('overviewPage.v2.redModelsTitle') }}</h2>
        <span class="ov-section-head__cap">{{ t('overviewPage.v2.redModelsCap') }}</span>
      </header>
      <div class="ov-red__tiles">
        <OverviewStatTile v-for="tile in modelTiles" :key="tile.id" v-bind="tile" @drill="$emit('drill', tile.id)" />
      </div>
    </div>
    <div class="ov-red__group">
      <header class="ov-section-head">
        <q-icon name="precision_manufacturing" size="18px" class="ov-section-head__icon" />
        <h2 class="ov-section-head__title">{{ t('overviewPage.v2.redRunsTitle') }}</h2>
        <span class="ov-section-head__cap">{{ t('overviewPage.v2.redRunsCap') }}</span>
        <q-btn-toggle
          :model-value="runWindow"
          dense
          outline
          no-caps
          class="ov-red__window"
          :options="runWindowOptions"
          @update:model-value="(v) => $emit('update:runWindow', Number(v))"
        />
      </header>
      <div class="ov-red__tiles ov-red__tiles--four">
        <OverviewStatTile v-for="tile in runTiles" :key="tile.id" v-bind="tile" @drill="$emit('drill', tile.id)" />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import OverviewStatTile from './OverviewStatTile.vue';
import type { OvRedTile } from './ovTypes';

defineProps<{
  modelTiles: OvRedTile[];
  runTiles: OvRedTile[];
  runWindow: number;
  runWindowOptions: { label: string; value: number }[];
}>();

defineEmits<{ 'update:runWindow': [v: number]; drill: [key: string] }>();

const { t } = useI18n();
</script>
