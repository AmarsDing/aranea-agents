<template>
  <!-- 派给 AI 的对象选择：放在按钮里面，点按钮弹出；可搜索，键盘上下选、回车确认，Esc 关闭回焦 -->
  <q-menu ref="menuEl" class="tk-pick" anchor="bottom right" self="top right" :offset="[0, 6]" @before-show="onShow">
    <div
      class="tk-pick__box"
      @keydown.down.prevent="move(1)"
      @keydown.up.prevent="move(-1)"
      @keydown.enter.prevent="pickActive"
    >
      <input
        ref="inputEl"
        v-model="query"
        class="tk-pick__search"
        type="search"
        autocomplete="off"
        :placeholder="t('taskBoard.dispatch.search')"
        :aria-label="t('taskBoard.dispatch.search')"
      />
      <p v-if="status === 'loading'" class="tk-pick__note">{{ t('taskBoard.dispatch.loading') }}</p>
      <p v-else-if="status === 'error'" class="tk-pick__note tk-pick__note--danger" role="alert">
        {{ t('taskBoard.dispatch.loadFailed') }}
      </p>
      <p v-else-if="filtered.length === 0" class="tk-pick__note">{{ t('taskBoard.dispatch.empty') }}</p>
      <ul v-else class="tk-pick__list" role="listbox" :aria-label="t('taskBoard.actions.dispatch')">
        <li
          v-for="(r, index) in filtered"
          :key="`${r.kind}:${r.id}`"
          v-close-popup
          role="option"
          class="tk-pick__item"
          :class="{ 'tk-pick__item--on': index === active }"
          :aria-selected="index === active"
          @click="emit('pick', r)"
          @mousemove="active = index"
        >
          <q-icon :name="r.kind === 'team' ? 'groups' : 'smart_toy'" size="18px" aria-hidden="true" />
          <span class="tk-pick__name">{{ r.name }}</span>
          <span class="tk-pick__kind">{{
            t(r.kind === 'team' ? 'taskBoard.dispatch.team' : 'taskBoard.dispatch.agent')
          }}</span>
        </li>
      </ul>
    </div>
  </q-menu>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import type { QMenu } from 'quasar';
import { useI18n } from 'vue-i18n';
import type { Recipient } from '../../features/today/command';
import { useTaskDispatch } from '../../features/tasks/useTaskDispatch';

const emit = defineEmits<{ pick: [recipient: Recipient] }>();

const { t } = useI18n();
const { recipients, recipientsStatus: status, ensureRecipients } = useTaskDispatch();

const query = ref('');
const active = ref(0);
const inputEl = ref<HTMLInputElement | null>(null);
const menuEl = ref<QMenu | null>(null);

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase();
  const rows = q
    ? recipients.value.filter((r) => r.name.toLowerCase().includes(q) || r.key.toLowerCase().includes(q))
    : recipients.value;
  return rows.slice(0, 50);
});

watch(filtered, () => (active.value = 0));

function onShow() {
  query.value = '';
  void ensureRecipients();
  void nextTick(() => inputEl.value?.focus());
}

function move(step: number) {
  const n = filtered.value.length;
  if (n) active.value = (active.value + step + n) % n;
}

function pickActive() {
  const target = filtered.value[active.value];
  if (!target) return;
  emit('pick', target);
  menuEl.value?.hide(); // 点击时由 v-close-popup 关
}
</script>
