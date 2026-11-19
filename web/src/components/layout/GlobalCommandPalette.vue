<template>
  <!--
    全局指令面板（Ctrl+K）：既能跳到任何页面，也能直接办事——交办、建任务、记一笔。
    与工作台指令栏共用同一套判断（features/today/command）与执行（useCommandRunner）。
  -->
  <q-dialog
    v-model="open"
    position="top"
    transition-show="fade"
    transition-hide="fade"
    class="app-command"
    @show="onShow"
  >
    <q-card class="app-command__card">
      <q-input
        ref="inputRef"
        v-model="query"
        class="app-command__input"
        borderless
        dense
        autofocus
        :disable="busy"
        :placeholder="t('shell.command.placeholder')"
        :aria-label="t('shell.command.placeholder')"
        role="combobox"
        aria-autocomplete="list"
        :aria-expanded="results.length > 0"
        @keydown.down.prevent="move(1)"
        @keydown.up.prevent="move(-1)"
        @keydown.enter.prevent="selectActive"
      >
        <template #prepend>
          <q-icon :name="busy ? 'hourglass_top' : 'bolt'" size="20px" />
        </template>
        <template #append>
          <kbd class="app-command__kbd">ESC</kbd>
        </template>
      </q-input>
      <q-separator />
      <p v-if="error" class="app-command__error" role="alert">
        {{ t('today.command.failed', { message: error }) }}
        <router-link v-if="errorLink" :to="errorLink" @click="open = false">{{
          t('today.command.openSession')
        }}</router-link>
      </p>
      <q-list v-if="results.length > 0" class="app-command__list" role="listbox">
        <template v-for="(entry, index) in results" :key="entry.key">
          <q-item-label v-if="entry.heading" header class="app-command__heading">{{ entry.heading }}</q-item-label>
          <q-item
            clickable
            :active="index === activeIndex"
            active-class="app-command__item--active"
            class="app-command__item"
            :class="{ 'app-command__item--action': entry.kind === 'action' }"
            role="option"
            :aria-selected="index === activeIndex"
            :disable="busy"
            @mouseenter="activeIndex = index"
            @click="select(entry)"
          >
            <q-item-section avatar>
              <q-icon :name="entry.icon" size="18px" />
            </q-item-section>
            <q-item-section>
              <q-item-label>{{ entry.label }}</q-item-label>
              <q-item-label caption class="app-command__caption">{{ entry.caption }}</q-item-label>
            </q-item-section>
            <q-item-section side>
              <span v-if="entry.badge" class="app-command__badge">{{ entry.badge }}</span>
              <span v-else-if="entry.path" class="app-command__path">{{ entry.path }}</span>
            </q-item-section>
          </q-item>
        </template>
      </q-list>
      <div v-else class="app-command__empty">{{ t('shell.command.empty') }}</div>
      <div class="app-command__footer">
        <span class="app-command__hint">↑ ↓ {{ t('shell.command.hintMove') }}</span>
        <span class="app-command__hint">↵ {{ t('shell.command.hintRun') }}</span>
        <span class="app-command__hint">{{ t('shell.command.hintSyntax') }}</span>
      </div>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { buildNavDomains } from '../../config/navDomains';
import { flatNavLeaves } from '../../config/navView';
import { planPaletteActions, type PaletteAction } from '../../features/today/command';
import { runCommand } from '../../features/today/useCommandRunner';
import { useRecipients } from '../../features/today/useDelegation';

const props = defineProps<{ modelValue: boolean; seed?: string }>();
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>();

const { t } = useI18n();
const $q = useQuasar();
const router = useRouter();
const query = ref('');
const activeIndex = ref(0);
const inputRef = ref<{ focus: () => void } | null>(null);
const busy = ref(false);
const error = ref('');
const errorLink = ref('');
const recipients = useRecipients();

const RECENT_KEY = 'nav:recent';
const MAX_RECENT = 5;
const MAX_PAGES = 40;

const open = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit('update:modelValue', value),
});

type NavEntry = {
  kind: 'nav';
  key: string;
  to: string;
  icon: string;
  label: string;
  caption: string;
  path: string;
  badge: string;
  heading: string;
};
type ActionEntry = {
  kind: 'action';
  key: string;
  action: PaletteAction;
  icon: string;
  label: string;
  caption: string;
  path: string;
  badge: string;
  heading: string;
};
type Entry = NavEntry | ActionEntry;

// 说明文字用新的一级去处名（与侧栏一致）；子页带上父页面名，例如「能力 · Tools 管理」
const domainCaption = computed(() => {
  const out = new Map<string, string>();
  for (const domain of buildNavDomains(() => true)) {
    for (const primary of domain.primaries) {
      out.set(primary.to, t(domain.labelKey));
      for (const child of primary.children) out.set(child.to, `${t(domain.labelKey)} · ${t(primary.tabLabelKey)}`);
    }
  }
  return out;
});

const pages = computed<NavEntry[]>(() =>
  flatNavLeaves().map((leaf) => ({
    kind: 'nav',
    key: `nav:${leaf.to}`,
    to: leaf.to,
    icon: leaf.icon,
    label: t(leaf.labelKey),
    caption: domainCaption.value.get(leaf.to) ?? '',
    path: leaf.to,
    badge: '',
    heading: '',
  })),
);

function loadRecent(): string[] {
  try {
    const raw = localStorage.getItem(RECENT_KEY);
    const parsed = raw ? (JSON.parse(raw) as unknown) : [];
    return Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === 'string') : [];
  } catch {
    return [];
  }
}

function remember(path: string) {
  try {
    const next = [path, ...loadRecent().filter((item) => item !== path)].slice(0, MAX_RECENT);
    localStorage.setItem(RECENT_KEY, JSON.stringify(next));
  } catch {
    // 隐私模式下 localStorage 不可写：忽略，不影响跳转
  }
}

function actionEntry(action: PaletteAction, index: number): ActionEntry {
  const base = {
    kind: 'action' as const,
    key: `act:${action.kind}:${index}`,
    action,
    caption: action.text,
    path: '',
    badge: '↵',
    heading: '',
  };
  if (action.kind === 'delegate') {
    return {
      ...base,
      icon: 'assignment_ind',
      label: t('shell.command.action.delegate', { name: action.recipient.name }),
    };
  }
  if (action.kind === 'note') return { ...base, icon: 'edit_note', label: t('shell.command.action.note') };
  return { ...base, icon: 'add_task', label: t('shell.command.action.task') };
}

function withHeading<T extends Entry>(list: T[], heading: string): T[] {
  return list.map((entry, i) => (i === 0 ? { ...entry, heading } : entry));
}

const results = computed<Entry[]>(() => {
  const keyword = query.value.trim().toLowerCase();
  if (!keyword) {
    const recents = loadRecent();
    const recentEntries = recents
      .map((path) => pages.value.find((entry) => entry.to === path))
      .filter((entry): entry is NavEntry => Boolean(entry))
      .map((entry) => ({ ...entry, badge: t('shell.command.recent') }));
    const rest = pages.value.filter((entry) => !recents.includes(entry.to));
    return [...recentEntries, ...rest].slice(0, MAX_PAGES);
  }
  const navHits = pages.value
    .filter((entry) => [entry.label, entry.caption, entry.to].some((field) => field.toLowerCase().includes(keyword)))
    .slice(0, MAX_PAGES);
  const plan = planPaletteActions(query.value, recipients.recipients.value);
  const actions = withHeading(plan.actions.map(actionEntry), t('shell.command.sectionDo'));
  const navs = withHeading(navHits, t('shell.command.sectionGo'));
  return plan.actionsFirst ? [...actions, ...navs] : [...navs, ...actions];
});

watch(query, () => {
  activeIndex.value = 0;
  error.value = '';
  errorLink.value = '';
});

watch(results, () => {
  if (activeIndex.value >= results.value.length) activeIndex.value = 0;
});

function onShow() {
  // 从右下角指令窗发出时带着一句话进来，其余情况为空
  query.value = props.seed ?? '';
  activeIndex.value = 0;
  error.value = '';
  errorLink.value = '';
  // 交办对象每次打开都刷新一次（读不到时只是少了交办选项，跳转照常）
  void recipients.load();
  void nextTick(() => inputRef.value?.focus());
}

function move(delta: number) {
  if (results.value.length === 0) return;
  const total = results.value.length;
  activeIndex.value = (activeIndex.value + delta + total) % total;
}

function selectActive() {
  const entry = results.value[activeIndex.value];
  if (entry) void select(entry);
}

async function select(entry: Entry) {
  if (busy.value) return;
  if (entry.kind === 'nav') {
    remember(entry.to);
    open.value = false;
    await router.push(entry.to);
    return;
  }
  await execute(entry.action);
}

async function execute(action: PaletteAction) {
  busy.value = true;
  const result = await runCommand(
    action.kind === 'delegate'
      ? { intent: 'delegate', text: action.text, recipient: action.recipient }
      : { intent: action.kind, text: action.text },
  );
  busy.value = false;
  if (!result.ok) {
    error.value = result.message;
    errorLink.value = result.link;
    void nextTick(() => inputRef.value?.focus());
    return;
  }
  open.value = false;
  const message =
    action.kind === 'delegate'
      ? t(result.untracked ? 'today.command.done.delegateUntracked' : 'today.command.done.delegate', {
          name: action.recipient.name,
        })
      : t(`today.command.done.${action.kind}`, { title: action.text });
  $q.notify({
    type: result.untracked ? 'warning' : 'positive',
    message,
    actions: result.link
      ? [{ label: t('today.command.openSession'), color: 'white', handler: () => void router.push(result.link) }]
      : [],
  });
}
</script>
