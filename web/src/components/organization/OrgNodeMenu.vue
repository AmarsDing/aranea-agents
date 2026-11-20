<template>
  <q-btn
    flat
    dense
    round
    size="sm"
    icon="more_horiz"
    class="org-menu-btn"
    :aria-label="t('orgChart.menu.more')"
    @click.stop
  >
    <q-menu anchor="bottom right" self="top right">
      <q-list dense class="org-menu">
        <q-item v-close-popup clickable @click="emit('edit')">
          <q-item-section avatar><q-icon name="edit" size="18px" /></q-item-section>
          <q-item-section>{{ t('orgChart.menu.edit') }}</q-item-section>
        </q-item>
        <q-item v-if="canUp" v-close-popup clickable @click="emit('move', -1)">
          <q-item-section avatar><q-icon name="arrow_upward" size="18px" /></q-item-section>
          <q-item-section>{{ t('orgChart.menu.moveUp') }}</q-item-section>
        </q-item>
        <q-item v-if="canDown" v-close-popup clickable @click="emit('move', 1)">
          <q-item-section avatar><q-icon name="arrow_downward" size="18px" /></q-item-section>
          <q-item-section>{{ t('orgChart.menu.moveDown') }}</q-item-section>
        </q-item>
        <template v-if="!system">
          <q-item v-close-popup clickable @click="emit('toggle')">
            <q-item-section avatar>
              <q-icon :name="node.enabled ? 'pause_circle' : 'play_circle'" size="18px" />
            </q-item-section>
            <q-item-section>{{ t(node.enabled ? 'orgChart.menu.disable' : 'orgChart.menu.enable') }}</q-item-section>
          </q-item>
          <q-separator />
          <q-item v-close-popup clickable class="org-menu__danger" @click="emit('remove')">
            <q-item-section avatar><q-icon name="delete" size="18px" /></q-item-section>
            <q-item-section>{{ t('orgChart.menu.remove') }}</q-item-section>
          </q-item>
        </template>
        <q-item v-else class="org-menu__note">
          <q-item-section>{{ t('orgChart.systemLocked') }}</q-item-section>
        </q-item>
      </q-list>
    </q-menu>
  </q-btn>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { PlatformResourceTreeNode } from '../../features/platform/types';

defineProps<{ node: PlatformResourceTreeNode; system: boolean; canUp?: boolean; canDown?: boolean }>();
const emit = defineEmits<{ edit: []; move: [delta: -1 | 1]; toggle: []; remove: [] }>();
const { t } = useI18n();
</script>
