<script setup lang="ts">
import type { Component } from 'vue'
import { Card, CardContent } from '@/components/ui/card'

/** Compact KPI tile: label, headline value, optional secondary line and icon. */
defineProps<{
  label: string
  value: string | number
  sub?: string
  icon?: Component
  /** Colours the secondary line, e.g. for a failure rate */
  subTone?: 'default' | 'positive' | 'negative' | 'warning'
}>()

const toneClass = {
  default: 'text-white/45 light:text-gray-500',
  positive: 'text-emerald-400 light:text-emerald-600',
  negative: 'text-red-400 light:text-red-600',
  warning: 'text-amber-400 light:text-amber-600',
}
</script>

<template>
  <Card>
    <CardContent class="p-5">
      <div class="flex items-center justify-between gap-3">
        <p class="text-[13px] font-medium text-white/60 light:text-gray-600 truncate">{{ label }}</p>
        <component :is="icon" v-if="icon" class="h-4 w-4 shrink-0 text-white/35 light:text-gray-400" />
      </div>
      <p class="mt-2 text-2xl font-semibold tracking-tight tabular-nums text-white light:text-gray-900 truncate">{{ value }}</p>
      <p v-if="sub" :class="['mt-1 text-xs truncate', toneClass[subTone || 'default']]">{{ sub }}</p>
    </CardContent>
  </Card>
</template>
