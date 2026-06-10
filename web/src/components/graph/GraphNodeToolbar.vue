<template>
  <div v-show="visible" class="graph-node-toolbar">
    <q-btn
      flat
      dense
      round
      icon="play_arrow"
      size="sm"
      class="graph-node-toolbar__btn"
      @click.stop="$emit('runToHere', nodeId)"
    >
      <q-tooltip :delay="500" :offset="[0, 4]">运行至此节点</q-tooltip>
    </q-btn>

    <q-btn
      flat
      dense
      round
      icon="ac_unit"
      size="sm"
      class="graph-node-toolbar__btn"
      :class="{ 'graph-node-toolbar__btn--active': frozen }"
      @click.stop="$emit('freeze', nodeId)"
    >
      <q-tooltip :delay="500" :offset="[0, 4]">{{ frozen ? '解冻节点' : '冻结节点（跳过执行）' }}</q-tooltip>
    </q-btn>

    <q-btn
      flat
      dense
      round
      icon="delete"
      size="sm"
      class="graph-node-toolbar__btn graph-node-toolbar__btn--danger"
      @click.stop="$emit('delete', nodeId)"
    >
      <q-tooltip :delay="500" :offset="[0, 4]">删除节点</q-tooltip>
    </q-btn>

    <q-btn flat dense round icon="more_horiz" size="sm" class="graph-node-toolbar__btn">
      <q-menu anchor="bottom middle" self="top middle" :offset="[0, 4]" transition-show="scale" transition-hide="scale">
        <q-list dense class="graph-node-toolbar__menu">
          <q-item v-close-popup clickable @click="$emit('duplicate', nodeId)">
            <q-item-section side>
              <q-icon name="content_copy" size="16px" />
            </q-item-section>
            <q-item-section>复制节点</q-item-section>
          </q-item>
          <q-item v-close-popup clickable @click="$emit('setEntry', nodeId)">
            <q-item-section side>
              <q-icon name="play_circle" size="16px" />
            </q-item-section>
            <q-item-section>设为入口节点</q-item-section>
          </q-item>
          <q-item v-close-popup clickable @click="$emit('setFinish', nodeId)">
            <q-item-section side>
              <q-icon name="stop_circle" size="16px" />
            </q-item-section>
            <q-item-section>设为终点节点</q-item-section>
          </q-item>
          <q-item v-close-popup clickable @click="$emit('disconnect', nodeId)">
            <q-item-section side>
              <q-icon name="link_off" size="16px" />
            </q-item-section>
            <q-item-section>断开所有连线</q-item-section>
          </q-item>
        </q-list>
      </q-menu>
    </q-btn>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  nodeId: string;
  frozen?: boolean;
  visible?: boolean;
}>();

defineEmits<{
  runToHere: [nodeId: string];
  freeze: [nodeId: string];
  delete: [nodeId: string];
  duplicate: [nodeId: string];
  setEntry: [nodeId: string];
  setFinish: [nodeId: string];
  disconnect: [nodeId: string];
}>();
</script>

<style scoped>
.graph-node-toolbar {
  position: absolute;
  bottom: calc(100% + 48px);
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 2px;
  height: 40px;
  padding: 0 6px;
  border-radius: 12px;
  background: var(--glass-elevated);
  border: 1px solid var(--glass-border);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
  box-shadow: 0 4px 16px color-mix(in srgb, var(--color-text-primary) 15%, transparent);
  z-index: 10;
  white-space: nowrap;
}

.graph-node-toolbar__btn {
  color: var(--color-text-secondary);
}

.graph-node-toolbar__btn:hover {
  color: var(--color-text-primary);
  background: color-mix(in srgb, var(--color-accent) 10%, transparent);
}

.graph-node-toolbar__btn--active {
  color: var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 12%, transparent);
}

.graph-node-toolbar__btn--danger:hover {
  color: var(--color-danger);
  background: color-mix(in srgb, var(--color-danger) 10%, transparent);
}

.graph-node-toolbar__menu {
  background: var(--glass-elevated);
  border: 1px solid var(--glass-border);
  border-radius: 8px;
  min-width: 180px;
}
</style>
