<template>
  <!--
    指令窗：平时是贴在页面底边正中的半圆按钮，点开向上浮起一条磨砂玻璃输入条（悬浮在内容上，不占版面）。
    它是「我的任务」唯一的输入口：写一句回车 / 点发送 = 记进我的任务；写「@某 Agent」或「让某某…」
    就建好立刻派给它；「记：…」记成随手记录。附件三种加法：回形针、拖进来、粘贴截图，
    缩略图平铺在输入条上方。跳转页面、搜索用顶栏的 Ctrl K。
    只有点收起或按 Esc 才收回；换页、提交、点别处都保持展开，展开与否记在本机。
  -->
  <div class="app-dock" @keydown.esc="collapse(true)">
    <Transition name="app-dock">
      <section
        v-if="open"
        ref="panelEl"
        class="app-dock__panel"
        :class="{ 'app-dock__panel--drag': dragging, 'app-dock__panel--files': files.length > 0 }"
        tabindex="-1"
        :aria-label="t('shell.nav.dock')"
        @dragenter.prevent="onDragEnter"
        @dragover.prevent
        @dragleave="onDragLeave"
        @drop.prevent="onDrop"
      >
        <ul v-if="files.length" class="app-dock__files" :aria-label="t('shell.nav.dockFiles', { n: files.length })">
          <li
            v-for="(item, index) in files"
            :key="item.key"
            class="app-dock__file"
            :class="{ 'app-dock__file--img': item.url }"
            :title="`${item.file.name} · ${fileSize(item.file.size)}`"
          >
            <img v-if="item.url" :src="item.url" alt="" />
            <template v-else>
              <q-icon :name="fileIcon(item.file.name, item.file.type)" size="14px" aria-hidden="true" />
              <span class="app-dock__file-name">{{ item.file.name }}</span>
            </template>
            <button
              type="button"
              class="app-dock__file-x"
              :aria-label="t('taskBoard.composer.remove', { name: item.file.name })"
              @click="removeFile(index)"
            >
              <q-icon name="close" size="12px" aria-hidden="true" />
            </button>
          </li>
        </ul>
        <form class="app-dock__form" @submit.prevent="send">
          <q-btn
            flat
            dense
            round
            size="sm"
            icon="attach_file"
            class="app-dock__tool"
            :aria-label="t('taskBoard.composer.attach')"
            :disable="busy"
            @click="fileEl?.click()"
          >
            <q-tooltip>{{ t('taskBoard.composer.attach') }}</q-tooltip>
          </q-btn>
          <input
            ref="inputEl"
            v-model="text"
            type="text"
            autocomplete="off"
            :placeholder="t('shell.nav.dockPlaceholder')"
            :aria-label="t('shell.nav.dockPlaceholder')"
            :disabled="busy"
            @paste="onPaste"
            @focus="ensureRecipients"
          />
          <q-btn
            unelevated
            dense
            round
            size="sm"
            color="primary"
            text-color="dark"
            icon="send"
            type="submit"
            class="app-dock__send"
            :aria-label="t('shell.nav.dockSend')"
            :loading="busy"
            :disable="!text.trim()"
          >
            <q-tooltip>{{ sendTip }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            size="sm"
            icon="open_in_full"
            class="app-dock__tool app-dock__assign"
            :aria-label="t('shell.nav.dockAssign')"
            :disable="busy"
            @click="openPanel"
          >
            <q-tooltip>{{ t('shell.nav.dockAssign') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            size="sm"
            icon="expand_more"
            class="app-dock__collapse"
            :aria-label="t('shell.nav.dockCollapse')"
            @click="collapse(true)"
          />
        </form>
        <p v-if="dragging" class="app-dock__drop" aria-hidden="true">{{ t('taskBoard.composer.dropHint') }}</p>
        <input
          ref="fileEl"
          type="file"
          multiple
          class="app-dock__picker"
          tabindex="-1"
          aria-hidden="true"
          @change="onPick"
        />
      </section>
    </Transition>
    <button
      v-show="!open"
      ref="tabEl"
      type="button"
      class="app-dock__tab"
      :aria-label="t('shell.nav.dock')"
      :title="t('shell.nav.dock')"
      @click="expand"
    >
      <q-icon name="bolt" size="20px" aria-hidden="true" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { openAssign } from '../../features/conversations/assign';
import { fileIcon, fileSize } from '../../features/tasks/board';
import { acceptFiles, OPEN_DOCK_EVENT, quickAdd, quickAddPlan } from '../../features/tasks/quickAdd';
import { useTaskDispatch } from '../../features/tasks/useTaskDispatch';

const STORE_KEY = 'aranea.dock.open';

const { t } = useI18n();
const $q = useQuasar();
const router = useRouter();
const { recipients, ensureRecipients, dispatch } = useTaskDispatch();

function readOpen(): boolean {
  try {
    return window.localStorage.getItem(STORE_KEY) === '1';
  } catch {
    return false;
  }
}

const open = ref(readOpen());
const text = ref('');
const files = ref<{ key: string; file: File; url: string }[]>([]);
const busy = ref(false);
const dragging = ref(false);
const panelEl = ref<HTMLElement | null>(null);
const inputEl = ref<HTMLInputElement | null>(null);
const tabEl = ref<HTMLButtonElement | null>(null);
const fileEl = ref<HTMLInputElement | null>(null);
let dragDepth = 0;

// 展开与否记在本机：换页（壳层重建本组件）、刷新后保持原样
watch(open, (value) => {
  try {
    window.localStorage.setItem(STORE_KEY, value ? '1' : '0');
  } catch {
    /* 隐私模式等写不进去时只在本次会话里保持 */
  }
});

// 页面底边只给半圆按钮留位；展开后的输入条悬浮在内容上，不再挤占版面
const root = document.documentElement;
const onOpenDock = () => void expand();
onMounted(() => {
  root.style.setProperty('--app-dock-space', '40px');
  window.addEventListener(OPEN_DOCK_EVENT, onOpenDock);
});
onBeforeUnmount(() => {
  root.style.removeProperty('--app-dock-space');
  window.removeEventListener(OPEN_DOCK_EVENT, onOpenDock);
  for (const item of files.value) if (item.url) URL.revokeObjectURL(item.url);
});

async function expand() {
  open.value = true;
  await nextTick();
  inputEl.value?.focus();
}

async function collapse(refocus: boolean) {
  if (!open.value) return;
  open.value = false;
  await nextTick();
  if (refocus) tabEl.value?.focus();
}

// 发送按钮的提示：这句话会落成什么（说明文字不再常驻在面板里）
const sendTip = computed(() => {
  const plan = quickAddPlan(text.value, recipients.value, files.value.length > 0);
  if (plan.kind === 'delegate') return t('taskBoard.composer.willDelegate', { name: plan.recipient?.name ?? '' });
  if (plan.kind === 'note') return t('shell.nav.dockWillNote');
  return t('shell.nav.dockSend');
});

function notify(message: string, ok: boolean, link = '') {
  $q.notify({
    type: ok ? 'positive' : 'negative',
    message,
    timeout: ok ? 4000 : 8000,
    actions: link
      ? [{ label: t('today.command.openSession'), color: 'white', handler: () => void router.push(link) }]
      : undefined,
  });
}

// ——— 附件 ———

function addFiles(list: FileList | File[] | null | undefined) {
  const { accepted, rejected } = acceptFiles(files.value.length, Array.from(list ?? []));
  for (const file of accepted) {
    const url = file.type.startsWith('image/') ? URL.createObjectURL(file) : '';
    files.value.push({ key: `${Date.now()}-${files.value.length}-${file.name}`, file, url });
  }
  const tooLarge = rejected.find((r) => r.reason === 'tooLarge');
  if (tooLarge) notify(t('taskBoard.composer.tooLarge', { name: tooLarge.name }), false);
  else if (rejected.length) notify(t('taskBoard.composer.tooMany', { n: 20 }), false);
}

function removeFile(index: number) {
  const [item] = files.value.splice(index, 1);
  if (item?.url) URL.revokeObjectURL(item.url);
}

function clearFiles() {
  for (const item of files.value) if (item.url) URL.revokeObjectURL(item.url);
  files.value = [];
}

function onPick(event: Event) {
  const input = event.target as HTMLInputElement;
  addFiles(input.files);
  input.value = '';
  inputEl.value?.focus();
}

function onPaste(event: ClipboardEvent) {
  const pasted = Array.from(event.clipboardData?.files ?? []);
  if (!pasted.length) return;
  event.preventDefault();
  // 截图粘贴进来没有像样的文件名：按时间起一个
  addFiles(
    pasted.map((f) =>
      f.name && f.name !== 'image.png'
        ? f
        : new File([f], `paste-${Date.now()}.${f.type.split('/')[1] || 'png'}`, { type: f.type }),
    ),
  );
}

function onDragEnter(event: DragEvent) {
  if (!event.dataTransfer?.types.includes('Files')) return;
  dragDepth += 1;
  dragging.value = true;
}

function onDragLeave() {
  dragDepth = Math.max(0, dragDepth - 1);
  if (!dragDepth) dragging.value = false;
}

function onDrop(event: DragEvent) {
  dragDepth = 0;
  dragging.value = false;
  addFiles(event.dataTransfer?.files);
  inputEl.value?.focus();
}

// 想选交给谁（同事、部门里的人、流程、编码工具）、选交回什么：带着已写的话和附件打开完整的交办面板
function openPanel() {
  openAssign({ text: text.value.trim(), files: files.value.map((item) => item.file) });
  text.value = '';
  clearFiles();
}

// ——— 发送：记进我的任务（@某 Agent 就建好立刻派给它；记：… 记成随手记录）———

async function send() {
  const raw = text.value.trim();
  if (!raw || busy.value) return;
  busy.value = true;
  try {
    await ensureRecipients();
    const outcome = await quickAdd(
      raw,
      files.value.map((item) => item.file),
      { recipients: recipients.value, dispatch: (task, recipient) => dispatch(task, recipient) },
    );
    if (!outcome.ok) {
      notify(t('taskBoard.dispatch.failed', { message: outcome.message }), false, outcome.link);
      return;
    }
    text.value = '';
    clearFiles();
    if (outcome.failedFiles.length)
      notify(t('taskBoard.composer.uploadFailed', { names: outcome.failedFiles.join('、') }), false);
    if (outcome.kind === 'delegate')
      notify(t('taskBoard.dispatch.ok', { name: outcome.recipient?.name ?? '' }), true, outcome.link);
    else notify(t(outcome.kind === 'note' ? 'shell.nav.dockNoted' : 'shell.nav.dockAdded'), true);
  } finally {
    busy.value = false;
    await nextTick();
    inputEl.value?.focus();
  }
}
</script>
