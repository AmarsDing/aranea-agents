<template>
  <!-- 液态玻璃 SVG 滤镜单例（M1）：全 Workbench 挂载一次，所有 GlassPanel 共享。
       kb-liquid-refract：装饰光纹位移（原 GlassPanel 内联，迁移至此）。
       kb-liquid-bg：背景真折射（backdrop-filter: url() 用，低频细腻位移）。 -->
  <svg width="0" height="0" aria-hidden="true" focusable="false" class="kb-liquid-defs">
    <defs>
      <filter id="kb-liquid-refract" color-interpolation-filters="sRGB">
        <feTurbulence type="fractalNoise" baseFrequency="0.012 0.028" numOctaves="2" seed="7" result="noise" />
        <feDisplacementMap in="SourceGraphic" in2="noise" scale="10" xChannelSelector="R" yChannelSelector="G" />
      </filter>
      <filter id="kb-liquid-bg" color-interpolation-filters="sRGB">
        <feTurbulence type="fractalNoise" baseFrequency="0.008 0.014" numOctaves="2" seed="11" result="noise" />
        <feDisplacementMap in="SourceGraphic" in2="noise" scale="14" xChannelSelector="R" yChannelSelector="G" />
      </filter>
    </defs>
  </svg>
</template>

<style lang="sass" scoped>
.kb-liquid-defs
  position: absolute
  pointer-events: none
</style>
