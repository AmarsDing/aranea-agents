<template>
  <div class="fleet-term">
    <div ref="host" class="fleet-term__host" @dragover.prevent @drop.prevent="onDrop" />
    <div v-if="banner" class="fleet-term__banner" role="status" aria-live="polite">
      <span>{{ banner }}</span>
      <q-btn v-if="exited" dense flat no-caps size="sm" icon="restart_alt" label="重启" @click="emit('restart')" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { Terminal as XTerm, type ITheme } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { buildFleetTerminalWsUrl } from '../../../config/runtime';

const props = defineProps<{ terminalId: string; active: boolean }>();
const emit = defineEmits<{ restart: []; jump: [] }>();

const host = ref<HTMLElement | null>(null);
const banner = ref('');
const exited = ref(false);

let term: XTerm | null = null;
let fit: FitAddon | null = null;
let ws: WebSocket | null = null;
let ro: ResizeObserver | null = null;
let retry: ReturnType<typeof setTimeout> | null = null;
let disposed = false;
let attempts = 0;

/** xterm 需要具体颜色值：运行时从主题令牌读取，组件里不写死任何色值。 */
function readTheme(): ITheme {
  const css = getComputedStyle(document.documentElement);
  const v = (name: string) => css.getPropertyValue(name).trim() || undefined;
  return {
    background: v('--nt-data-inset') ?? v('--nt-bg-0'),
    foreground: v('--nt-text-1'),
    cursor: v('--nt-accent'),
    cursorAccent: v('--nt-bg-0'),
    selectionBackground: v('--nt-accent-soft'),
  };
}

function send(msg: object) {
  if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg));
}

function sendSize() {
  if (!term || !fit) return;
  try {
    fit.fit();
  } catch {
    return;
  }
  send({ type: 'resize', cols: term.cols, rows: term.rows });
}

function connect() {
  if (disposed) return;
  ws = new WebSocket(buildFleetTerminalWsUrl(props.terminalId));
  ws.binaryType = 'arraybuffer';
  let first = true;
  ws.onopen = () => {
    attempts = 0;
    banner.value = '';
    sendSize();
  };
  ws.onmessage = (ev) => {
    if (typeof ev.data === 'string') {
      try {
        const m = JSON.parse(ev.data) as { type?: string; code?: number };
        if (m.type === 'exit') {
          exited.value = true;
          banner.value = m.code === 0 ? '进程已退出' : `进程异常退出（${m.code}）`;
        }
      } catch {
        /* 非控制帧忽略 */
      }
      return;
    }
    if (first) {
      // 重连时服务端会整段回放缓冲：先清屏，避免重复。
      term?.reset();
      first = false;
    }
    term?.write(new Uint8Array(ev.data as ArrayBuffer));
  };
  ws.onclose = (ev) => {
    if (disposed || exited.value) return;
    if (ev.code === 1006 || ev.code === 1000 || ev.code === 1001) {
      attempts++;
      if (attempts > 8) {
        banner.value = '连接已断开';
        return;
      }
      banner.value = attempts > 1 ? '正在重连…' : '';
      retry = setTimeout(connect, Math.min(4000, 300 * attempts));
    } else {
      banner.value = `连接被拒绝（${ev.code}）`;
    }
  };
}

function onDrop(ev: DragEvent) {
  // 把文件（路径文本）拖进终端：粘贴路径。
  const text = ev.dataTransfer?.getData('text/plain');
  if (text) send({ type: 'input', data: text });
}

onMounted(() => {
  term = new XTerm({
    fontFamily: getComputedStyle(document.documentElement).getPropertyValue('--nt-font-mono').trim() || 'monospace',
    fontSize: 13,
    lineHeight: 1.15,
    cursorBlink: true,
    scrollback: 8000,
    allowProposedApi: false,
    theme: readTheme(),
  });
  fit = new FitAddon();
  term.loadAddon(fit);
  term.open(host.value!);
  term.onData((data) => send({ type: 'input', data }));
  // 运行台全局快捷键不被终端吞掉。
  term.attachCustomKeyEventHandler((e) => !(e.ctrlKey && !e.shiftKey && !e.altKey && (e.key === 'j' || e.key === 'J')));
  ro = new ResizeObserver(() => sendSize());
  ro.observe(host.value!);
  connect();
  if (props.active) term.focus();
});

watch(
  () => props.active,
  (a) => {
    if (a) {
      requestAnimationFrame(() => {
        sendSize();
        term?.focus();
      });
    }
  },
);

watch(
  () => props.terminalId,
  () => {
    exited.value = false;
    ws?.close();
    connect();
  },
);

/** 外部（重启后）要求重新接入。 */
function reconnect() {
  exited.value = false;
  banner.value = '';
  ws?.close();
  connect();
}
defineExpose({ reconnect, focus: () => term?.focus() });

onBeforeUnmount(() => {
  disposed = true;
  if (retry) clearTimeout(retry);
  ro?.disconnect();
  ws?.close();
  term?.dispose();
});
</script>

<style scoped lang="sass">
.fleet-term
  position: relative
  height: 100%
  min-height: 0
  background: var(--nt-data-inset)
.fleet-term__host
  position: absolute
  inset: var(--nt-space-1) 0 0 var(--nt-space-2)
.fleet-term__banner
  position: absolute
  right: var(--nt-space-3)
  bottom: var(--nt-space-3)
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  padding: var(--nt-space-1) var(--nt-space-3)
  border: 1px solid var(--nt-border-2)
  border-radius: var(--nt-radius-m)
  background: var(--nt-bg-2)
  color: var(--nt-text-1)
  font-size: var(--nt-fs-caption)
</style>
