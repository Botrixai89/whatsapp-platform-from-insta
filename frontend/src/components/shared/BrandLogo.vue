<script setup lang="ts">
/**
 * BotrixAI brand logo: the green mark, optionally followed by the wordmark.
 * The wordmark uses the current text colour so it works in dark and light themes.
 */
withDefaults(defineProps<{
  /** Tailwind size classes for the mark */
  markClass?: string
  /** Show the "BotrixAI" wordmark next to the mark */
  showText?: boolean
  /** Tailwind classes for the wordmark */
  textClass?: string
}>(), {
  markClass: 'h-7 w-7',
  showText: true,
  textClass: 'text-sm font-semibold',
})
</script>

<template>
  <span class="inline-flex items-center gap-2">
    <svg :class="['shrink-0', markClass]" viewBox="0 0 410 386" fill="none" aria-hidden="true">
      <path
        d="M242.4 37.6 Q284.0 18.0 322.4 43.3 L324.9 44.9 Q360.0 68.0 365.8 109.6 L378.1 198.5 Q386.0 256.0 336.5 286.3 L259.2 333.7 Q208.0 365.0 149.2 353.1 L143.0 351.9 Q94.0 342.0 70.6 297.8 L40.7 241.4 Q22.0 206.0 36.9 168.9 L39.1 163.1 Q54.0 126.0 90.2 109.0 Z"
        stroke="#14A06F"
        stroke-width="30"
        stroke-linejoin="round"
      />
    </svg>
    <span v-if="showText" :class="['tracking-tight', textClass]">BotrixAI</span>
  </span>
</template>
