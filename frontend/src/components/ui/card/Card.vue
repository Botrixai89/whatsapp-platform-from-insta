<script setup lang="ts">
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: string
}>()
</script>

<template>
  <div
    :class="cn(
      'rounded-xl border border-white/[0.08] bg-white/[0.02] text-card-foreground light:bg-white light:border-gray-200 light:shadow-sm',
      props.class
    )"
  >
    <slot />
  </div>
</template>
