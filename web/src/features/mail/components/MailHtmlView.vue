<template>
  <div class="ml-html">
    <div v-if="blocked && !showImages" class="ml-html__bar" role="status">
      <q-icon name="image_not_supported" size="16px" aria-hidden="true" />
      <span>{{ t('mailPage.imagesBlocked', { n: blocked }) }}</span>
      <q-btn flat dense no-caps size="sm" :label="t('mailPage.showImages')" @click="showImages = true" />
    </div>
    <!-- 不给 allow-scripts：信里的脚本一律不跑；allow-same-origin 只为量高度（没脚本就拿不到权限） -->
    <iframe
      ref="frame"
      class="ml-html__frame"
      :title="title"
      sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
      referrerpolicy="no-referrer"
      :srcdoc="doc"
      :style="{ height: `${height}px` }"
      @load="measure"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { countBlockedImages, mailSrcdoc, type MailPaper } from '../model';

const props = defineProps<{ html: string; title: string }>();
const { t } = useI18n();

const frame = ref<HTMLIFrameElement | null>(null);
const showImages = ref(false);
const height = ref(240);
let observer: ResizeObserver | null = null;

watch(
  () => props.html,
  () => {
    showImages.value = false;
  },
);

const blocked = computed(() => countBlockedImages(props.html));

/** 信是按白纸写的：纸面 / 墨色取令牌的计算值（--nt-ink-inverse / --nt-ink）。 */
function paper(): MailPaper {
  const cs = getComputedStyle(document.documentElement);
  const v = (name: string) => cs.getPropertyValue(name).trim();
  return {
    paper: v('--nt-ink-inverse'),
    ink: v('--nt-ink'),
    link: v('--nt-line-2'),
    muted: v('--nt-text-3'),
    font: v('--nt-font-sans') || 'sans-serif',
  };
}

const doc = computed(() => mailSrcdoc(props.html, showImages.value, paper()));

function measure() {
  const body = frame.value?.contentDocument?.body;
  if (!body) return;
  const update = () => {
    height.value = Math.max(120, Math.ceil(body.scrollHeight) + 4);
  };
  update();
  observer?.disconnect();
  observer = new ResizeObserver(update);
  observer.observe(body);
}

onBeforeUnmount(() => observer?.disconnect());
</script>
