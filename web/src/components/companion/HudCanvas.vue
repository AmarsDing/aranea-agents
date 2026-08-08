<template>
  <div ref="hostRef" class="hud-canvas" @click="emit('toggleChat')">
    <canvas ref="canvasRef" class="hud-canvas__gl" />
    <div v-if="glFailed" class="hud-canvas__fallback column flex-center">
      <q-icon name="blur_on" size="64px" class="hud-canvas__fallback-icon" />
      <div class="hud-canvas__fallback-text">{{ t('companion.webglUnsupported') }}</div>
    </div>

    <!-- 状态指示（需求 §2.3：聆听态出现「正在聆听…」；V5-T4 全息化：角括号 + 扫描线 + 等宽大写发光） -->
    <div v-if="stateLabel" class="hud-canvas__state" role="status" aria-live="polite">
      <span class="hud-canvas__state-corner hud-canvas__state-corner--tl" />
      <span class="hud-canvas__state-corner hud-canvas__state-corner--tr" />
      <span class="hud-canvas__state-corner hud-canvas__state-corner--bl" />
      <span class="hud-canvas__state-corner hud-canvas__state-corner--br" />
      <span class="hud-canvas__state-scan" />
      <q-icon v-if="voiceState === 'listening'" name="graphic_eq" size="14px" class="q-mr-xs" />
      <span class="hud-canvas__state-text">{{ stateLabel }}</span>
    </div>

    <!-- 实时字幕（需求 §2.4：字幕浮现在 HUD 下方，transient 不落消息流；V5-T4 打字机逐字浮现） -->
    <transition name="hud-subtitle">
      <div v-if="subtitle" class="hud-canvas__subtitle">
        <span v-for="(ch, i) in subtitleChars" :key="i" class="hud-canvas__subtitle-char">{{ ch }}</span>
      </div>
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

    <!-- 麦克风按钮（语音模式开关；需求 §2.3：点击进入语音模式）。
         V2-T8 差距2：voiceDisabled（语音服务未配置）时置灰阻断，tooltip 指引配置入口。
         禁用按钮 pointer-events:none，tooltip 须挂在外层 span 上才能悬停可见。
         V5-T4 反应堆式：脉动光环常开，语音模式开启时外圈轨道环旋转。 -->
    <span
      class="hud-canvas__mic-wrap"
      :class="{ 'hud-canvas__mic-wrap--on': voiceModeOn, 'hud-canvas__mic-wrap--disabled': voiceDisabled }"
    >
      <span class="hud-canvas__mic-halo" />
      <span class="hud-canvas__mic-orbit" />
      <q-btn
        round
        class="hud-canvas__mic"
        :class="{ 'hud-canvas__mic--on': voiceModeOn }"
        :icon="voiceDisabled ? 'mic_off' : voiceModeOn ? 'mic' : 'mic_none'"
        :disable="voiceDisabled"
        :aria-label="micAriaLabel"
        :aria-pressed="voiceModeOn"
        @click.stop="emit('toggleVoice')"
      />
      <q-tooltip>{{ micAriaLabel }}</q-tooltip>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';

import { HudScene, type AvatarRenderer } from '../../features/companion/hud/HudScene';
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
  /** V2-T8 差距2：语音服务未配置时麦克风置灰门控（父级经 /v1/voice/status 探测）。 */
  voiceDisabled?: boolean;
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

/** 麦克风按钮文案（aria-label + tooltip 共用）：置灰时指引配置入口。 */
const micAriaLabel = computed(() => {
  if (props.voiceDisabled) return t('companion.voiceUnavailable');
  return props.voiceModeOn ? t('companion.micStop') : t('companion.micStart');
});

/** V5-T4 打字机字幕：逐字 span，新到字符单独触发浮现动画（按索引复用，旧字不重放）。 */
const subtitleChars = computed(() => props.subtitle.split(''));

onMounted(() => {
  const host = hostRef.value;
  const canvas = canvasRef.value;
  if (!host || !canvas) return;
  try {
    // V5 拉取模型：音频数据源经 provider 回调注入，场景每帧自取，无需 watch 推送。
    scene = new HudScene(canvas, {
      getPlaybackLevel: () => props.amplitude,
      fillMicSpectrum: (bins: Uint8Array) => {
        const s = props.spectrum;
        if (!s || s.length === 0) {
          bins.fill(0);
          return;
        }
        bins.fill(0);
        bins.set(s.subarray(0, Math.min(s.length, bins.length)));
      },
    });
  } catch {
    glFailed.value = true;
    return;
  }
  scene.setState(props.voiceState);
  scene.setVoiceMode(props.voiceModeOn);
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
// V5-T3 启动过场：语音模式开启时推进反应堆点亮序列
watch(
  () => props.voiceModeOn,
  (on) => scene?.setVoiceMode(on),
);

/** V2-T5：确认通过等外部触发的一次性能量脉冲（核闪光 + 涟漪）。 */
function triggerBurst(): void {
  scene?.burst();
}

defineExpose({ triggerBurst });
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
    padding: 7px 20px
    font-size: 12px
    letter-spacing: 0.22em
    text-transform: uppercase
    font-family: var(--font-mono, monospace)
    color: var(--color-neon-cyan)
    text-shadow: 0 0 8px rgba(0, 229, 255, 0.6)
    background: rgba(9, 13, 20, 0.55)
    backdrop-filter: blur(var(--glass-blur-default))
    -webkit-backdrop-filter: blur(var(--glass-blur-default))
    border: 1px solid rgba(0, 229, 255, 0.35)
    box-shadow: 0 0 16px rgba(0, 229, 255, 0.15), inset 0 0 18px rgba(0, 229, 255, 0.05)
    overflow: hidden

  &__state-corner
    position: absolute
    width: 7px
    height: 7px
    border: 1.5px solid rgba(0, 229, 255, 0.85)
    filter: drop-shadow(0 0 3px rgba(0, 229, 255, 0.6))
    pointer-events: none

    &--tl
      top: 2px
      left: 2px
      border-right: none
      border-bottom: none
    &--tr
      top: 2px
      right: 2px
      border-left: none
      border-bottom: none
    &--bl
      bottom: 2px
      left: 2px
      border-right: none
      border-top: none
    &--br
      bottom: 2px
      right: 2px
      border-left: none
      border-top: none

  &__state-scan
    position: absolute
    inset: 0
    pointer-events: none
    background: linear-gradient(180deg, transparent 0%, rgba(0, 229, 255, 0.1) 48%, rgba(0, 229, 255, 0.22) 50%, rgba(0, 229, 255, 0.1) 52%, transparent 100%)
    background-size: 100% 240%
    animation: hud-scan 2.6s linear infinite

  &__subtitle
    position: absolute
    bottom: 96px
    left: 50%
    transform: translateX(-50%)
    max-width: min(560px, 80%)
    padding: 10px 18px
    border-radius: 4px
    font-size: 15px
    line-height: 1.5
    text-align: center
    color: var(--color-text-primary)
    text-shadow: 0 0 6px rgba(0, 229, 255, 0.35)
    background: rgba(9, 13, 20, 0.6)
    backdrop-filter: blur(var(--glass-blur-default))
    -webkit-backdrop-filter: blur(var(--glass-blur-default))
    border: 1px solid rgba(0, 229, 255, 0.3)
    box-shadow: 0 0 18px rgba(0, 229, 255, 0.12), inset 0 0 24px rgba(0, 229, 255, 0.04)

  &__subtitle-char
    display: inline-block
    white-space: pre
    animation: hud-char-in 0.18s ease-out both

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

  &__mic-wrap
    position: absolute
    bottom: 28px
    left: 50%
    transform: translateX(-50%)
    display: inline-flex

  // 反应堆脉动光环（常开；V5-T4）
  &__mic-halo
    position: absolute
    inset: -5px
    border-radius: 50%
    border: 1px solid rgba(0, 229, 255, 0.45)
    pointer-events: none
    animation: hud-mic-pulse 2.4s ease-out infinite

  // 外圈轨道环（语音模式开启时旋转）
  &__mic-orbit
    position: absolute
    inset: -11px
    border-radius: 50%
    border: 1px dashed rgba(0, 229, 255, 0.55)
    pointer-events: none
    opacity: 0
    transition: opacity 0.3s ease

  &__mic-wrap--on &__mic-orbit
    opacity: 1
    animation: hud-mic-orbit 6s linear infinite

  &__mic-wrap--disabled &__mic-halo
    display: none

  &__mic
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

// V5-T4 全息化动画
@keyframes hud-scan
  from
    background-position: 0 -120%
  to
    background-position: 0 120%

@keyframes hud-char-in
  from
    opacity: 0
    transform: translateY(4px)
    text-shadow: 0 0 10px rgba(0, 229, 255, 0.9)
  to
    opacity: 1
    transform: translateY(0)

@keyframes hud-mic-pulse
  0%
    transform: scale(0.92)
    opacity: 0.7
  70%
    transform: scale(1.4)
    opacity: 0
  100%
    transform: scale(1.4)
    opacity: 0

@keyframes hud-mic-orbit
  from
    transform: rotate(0deg)
  to
    transform: rotate(360deg)
</style>
