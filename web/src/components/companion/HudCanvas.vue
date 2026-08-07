<template>
  <div ref="hostRef" class="hud-canvas" @click="emit('toggleChat')">
    <canvas ref="canvasRef" class="hud-canvas__gl" />
    <div v-if="glFailed" class="hud-canvas__fallback column flex-center">
      <q-icon name="blur_on" size="64px" class="hud-canvas__fallback-icon" />
      <div class="hud-canvas__fallback-text">{{ t('companion.webglUnsupported') }}</div>
    </div>

    <!-- 状态指示（需求 §2.3：聆听态出现「正在聆听…」） -->
    <div v-if="stateLabel" class="hud-canvas__state" role="status" aria-live="polite">
      <q-icon v-if="voiceState === 'listening'" name="graphic_eq" size="14px" class="q-mr-xs" />
      {{ stateLabel }}
    </div>

    <!-- 实时字幕（需求 §2.4：字幕浮现在 HUD 下方，transient 不落消息流） -->
    <transition name="hud-subtitle">
      <div v-if="subtitle" class="hud-canvas__subtitle">{{ subtitle }}</div>
    </transition>

    <!-- 降级错误条（voice.error / 本地采集错误） -->
    <q-banner v-if="error" dense rounded class="hud-canvas__error" @click.stop>
      <template #avatar>
        <q-icon name="mic_off" size="18px" />
      </template>
      {{ error.message }}
      <template #action>
        <q-btn flat dense size="sm" :label="t('companion.dismiss')" @click="emit('dismissError')" />
      </template>
    </q-banner>

    <!-- 麦克风按钮（语音模式开关；需求 §2.3：点击进入语音模式） -->
    <q-btn
      round
      class="hud-canvas__mic"
      :class="{ 'hud-canvas__mic--on': voiceModeOn }"
      :icon="voiceModeOn ? 'mic' : 'mic_none'"
      :aria-label="voiceModeOn ? t('companion.micStop') : t('companion.micStart')"
      :aria-pressed="voiceModeOn"
      @click.stop="emit('toggleVoice')"
    >
      <q-tooltip>{{ voiceModeOn ? t('companion.micStop') : t('companion.micStart') }}</q-tooltip>
    </q-btn>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';

import { createHudScene, type AvatarRenderer } from '../../features/companion/hud/HudScene';
import type { VoiceError, VoiceState } from '../../features/companion/types';

const props = defineProps<{
  voiceState: VoiceState;
  voiceModeOn: boolean;
  subtitle: string;
  error: VoiceError | null;
  /** listening 态采集侧 FFT 数据（频谱环）。 */
  spectrum: Uint8Array | null;
  /** speaking 态播放振幅 [0,1]（能量核脉动）。 */
  amplitude: number;
}>();

const emit = defineEmits<{
  toggleChat: [];
  toggleVoice: [];
  dismissError: [];
}>();

const { t } = useI18n();

const hostRef = ref<HTMLDivElement | null>(null);
const canvasRef = ref<HTMLCanvasElement | null>(null);
const glFailed = ref(false);

let scene: AvatarRenderer | null = null;
let resizeObserver: ResizeObserver | null = null;

const stateLabel = computed(() => {
  if (!props.voiceModeOn) return '';
  switch (props.voiceState) {
    case 'listening':
      return t('companion.stateListening');
    case 'thinking':
      return t('companion.stateThinking');
    case 'speaking':
      return t('companion.stateSpeaking');
    case 'interrupted':
      return t('companion.stateInterrupted');
    default:
      return '';
  }
});

onMounted(() => {
  const host = hostRef.value;
  const canvas = canvasRef.value;
  if (!host || !canvas) return;
  try {
    scene = createHudScene(canvas);
  } catch {
    glFailed.value = true;
    return;
  }
  scene.setState(props.voiceState);
  scene.resize(host.clientWidth, host.clientHeight);
  resizeObserver = new ResizeObserver((entries) => {
    const box = entries[0]?.contentRect;
    if (box) scene?.resize(Math.round(box.width), Math.round(box.height));
  });
  resizeObserver.observe(host);
});

onBeforeUnmount(() => {
  resizeObserver?.disconnect();
  resizeObserver = null;
  scene?.dispose();
  scene = null;
});

watch(
  () => props.voiceState,
  (s) => scene?.setState(s),
);
watch(
  () => props.amplitude,
  (v) => scene?.setAmplitude(v),
);
watch(
  () => props.spectrum,
  (data) => scene?.setSpectrum(data),
);
</script>

<style scoped lang="sass">
.hud-canvas
  position: relative
  width: 100%
  height: 100%
  overflow: hidden
  cursor: pointer

  &__gl
    position: absolute
    inset: 0
    width: 100%
    height: 100%
    display: block

  &__fallback
    position: absolute
    inset: 0
    gap: 12px

  &__fallback-icon
    color: var(--color-neon-cyan)
    opacity: 0.6

  &__fallback-text
    color: var(--color-text-secondary)
    font-size: 13px

  &__state
    position: absolute
    top: 24px
    left: 50%
    transform: translateX(-50%)
    display: flex
    align-items: center
    padding: 6px 14px
    border-radius: 999px
    font-size: 12px
    letter-spacing: 0.08em
    color: var(--color-neon-cyan)
    background: rgba(9, 13, 20, 0.55)
    backdrop-filter: blur(var(--glass-blur-default))
    -webkit-backdrop-filter: blur(var(--glass-blur-default))
    border: 1px solid rgba(0, 229, 255, 0.25)

  &__subtitle
    position: absolute
    bottom: 96px
    left: 50%
    transform: translateX(-50%)
    max-width: min(560px, 80%)
    padding: 10px 18px
    border-radius: 14px
    font-size: 15px
    line-height: 1.5
    text-align: center
    color: var(--color-text-primary)
    background: rgba(9, 13, 20, 0.6)
    backdrop-filter: blur(var(--glass-blur-default))
    -webkit-backdrop-filter: blur(var(--glass-blur-default))
    border: 1px solid var(--glass-border)

  &__error
    position: absolute
    top: 64px
    left: 50%
    transform: translateX(-50%)
    max-width: 90%
    border: 1px solid var(--color-warning)
    color: var(--color-text-primary)
    background: rgba(9, 13, 20, 0.72)
    backdrop-filter: blur(var(--glass-blur-default))
    -webkit-backdrop-filter: blur(var(--glass-blur-default))

  &__mic
    position: absolute
    bottom: 28px
    left: 50%
    transform: translateX(-50%)
    color: var(--color-neon-cyan)
    background: rgba(9, 13, 20, 0.6)
    border: 1px solid rgba(0, 229, 255, 0.35)
    backdrop-filter: blur(var(--glass-blur-default))
    -webkit-backdrop-filter: blur(var(--glass-blur-default))
    transition: box-shadow 0.25s ease, background 0.25s ease

    &--on
      background: rgba(0, 229, 255, 0.18)
      box-shadow: 0 0 18px rgba(0, 229, 255, 0.35)

.hud-subtitle-enter-active,
.hud-subtitle-leave-active
  transition: opacity 0.2s ease, transform 0.2s ease

.hud-subtitle-enter-from,
.hud-subtitle-leave-to
  opacity: 0
  transform: translateX(-50%) translateY(6px)
</style>
