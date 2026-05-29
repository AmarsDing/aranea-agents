// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <q-card flat bordered class="memory-card">
    <q-card-section>
      <div class="text-h6">记忆设置实施状态</div>
      <div class="text-caption text-grey-7">Agent 设置页已存在基础记忆字段；完整 L0–L4 设置 Tab 将按 `docs/需求/memory.md` 与 `12`–`16` memory-L*-*.md 继续扩展。</div>
    </q-card-section>
    <q-list separator>
      <q-item v-for="item in items" :key="item.label">
        <q-item-section avatar>
          <q-icon :name="item.done ? 'check_circle' : 'radio_button_unchecked'" :color="item.done ? 'positive' : 'grey'" />
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
defineProps<{
  items: Array<{
    label: string;
    caption: string;
    done: boolean;
  }>;
}>();
</script>
