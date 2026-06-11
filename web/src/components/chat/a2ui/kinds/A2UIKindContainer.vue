<template>
  <template v-if="kind === 'Card'">
    <q-card flat bordered class="a2ui-card">
      <q-card-section v-if="ctx.cardChildId">
        <A2UIComponentNode
          :component-id="ctx.cardChildId"
          :surface="surface"
          @user-action="(p) => emit('user-action', p)"
        />
      </q-card-section>
    </q-card>
  </template>
  <template v-else-if="kind === 'Modal'">
    <div class="a2ui-modal-entry" @click="emit('update:modalOpen', true)">
      <A2UIComponentNode
        v-if="ctx.modalEntryId"
        :component-id="ctx.modalEntryId"
        :surface="surface"
        @user-action="(p) => emit('user-action', p)"
      />
    </div>
    <q-dialog :model-value="ctx.modalOpen" @update:model-value="emit('update:modalOpen', $event)">
      <q-card class="a2ui-modal-card app-dialog-card">
        <q-card-section>
          <A2UIComponentNode
            v-if="ctx.modalContentId"
            :component-id="ctx.modalContentId"
            :surface="surface"
            @user-action="(p) => emit('user-action', p)"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat :label="closeLabel" @click="emit('update:modalOpen', false)" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </template>
  <template v-else-if="kind === 'Tabs'">
    <q-tabs :model-value="ctx.activeTab" @update:model-value="emit('update:activeTab', Number($event))" dense class="text-grey-7" active-color="accent" indicator-color="accent">
      <q-tab v-for="(tab, idx) in ctx.tabItems" :key="idx" :name="idx" :label="tab.label" />
    </q-tabs>
    <q-tab-panels :model-value="ctx.activeTab" @update:model-value="emit('update:activeTab', Number($event))" animated>
      <q-tab-panel v-for="(tab, idx) in ctx.tabItems" :key="idx" :name="idx">
        <A2UIComponentNode
          v-if="tab.childId"
          :component-id="tab.childId"
          :surface="surface"
          @user-action="(p) => emit('user-action', p)"
        />
      </q-tab-panel>
    </q-tab-panels>
  </template>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { A2UIComponentCtx } from '../../../../features/chat/a2ui/useA2UIComponent';
import type { A2UISurfaceState } from '../../../../features/chat/a2uiSurfaceState';
import type { A2UIUserActionPayload } from '../../../../features/chat/a2uiUserAction';
import A2UIComponentNode from '../../A2UIComponentNode.vue';

defineProps<{
  kind: string;
  ctx: A2UIComponentCtx;
  surface: A2UISurfaceState;
}>();

const emit = defineEmits<{
  'user-action': [payload: A2UIUserActionPayload];
  'update:modalOpen': [value: boolean];
  'update:activeTab': [value: number];
}>();

const { t } = useI18n();
const closeLabel = computed(() => t('common.close', '关闭'));
</script>

<style scoped>
.a2ui-card {
  background: var(--glass-elevated);
  border-color: var(--glass-border);
}

.a2ui-modal-entry {
  cursor: pointer;
}

.a2ui-modal-card {
  min-width: min(280px, 90vw);
  max-width: 90vw;
}
</style>
