<template>
  <!--
    右下角浮动的指令窗：默认打开，可以收起成一个按钮。
    它只负责收一句话，交给全局指令面板去判断和执行（跳转、交办、建任务、记一笔）。
  -->
  <section v-if="open" class="app-dock" :aria-label="t('shell.nav.dock')">
    <header class="app-dock__head">
      <q-icon name="bolt" size="18px" aria-hidden="true" />
      <span>{{ t('shell.nav.dock') }}</span>
      <kbd>{{ t('shell.command.hint') }}</kbd>
      <q-btn
        flat
        dense
        round
        size="sm"
        icon="expand_more"
        :aria-label="t('shell.nav.dockCollapse')"
        @click="open = false"
      />
    </header>
    <form class="app-dock__form" @submit.prevent="submit">
      <input
        v-model="text"
        type="text"
        autocomplete="off"
        :placeholder="t('shell.nav.dockPlaceholder')"
        :aria-label="t('shell.nav.dockPlaceholder')"
      />
      <q-btn
        unelevated
        dense
        color="primary"
        text-color="dark"
        icon="send"
        type="submit"
        :aria-label="t('shell.nav.dockSend')"
      />
    </form>
  </section>
  <q-btn
    v-else
    class="app-dock-fab"
    unelevated
    rounded
    color="primary"
    text-color="dark"
    icon="bolt"
    :label="t('shell.nav.dock')"
    @click="open = true"
  />
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';

const emit = defineEmits<{ submit: [text: string] }>();

const { t } = useI18n();

// 收起 / 打开按人记住；读写不了本地存储时只影响记忆，不影响使用
const KEY = 'nav:dock';
function read(): boolean {
  try {
    return localStorage.getItem(KEY) !== '0';
  } catch {
    return true;
  }
}
const open = ref(read());
// 告诉页面右下角被占了多高，一屏布局的页面（如工作台今日）据此给最右一列留出位置
const root = document.documentElement;
watch(
  open,
  (value) => {
    root.style.setProperty('--app-dock-space', value ? '128px' : '56px');
    root.style.setProperty('--app-dock-w', value ? '356px' : '132px');
  },
  { immediate: true },
);
onBeforeUnmount(() => {
  root.style.removeProperty('--app-dock-space');
  root.style.removeProperty('--app-dock-w');
});
watch(open, (value) => {
  try {
    localStorage.setItem(KEY, value ? '1' : '0');
  } catch {
    // 存不了不影响本次会话
  }
});

const text = ref('');
function submit() {
  emit('submit', text.value.trim());
  text.value = '';
}
</script>
