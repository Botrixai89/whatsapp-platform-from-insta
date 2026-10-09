<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  modelValue?: string
  placeholder?: string
  disabled?: boolean
  rows?: number
  class?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const modelValue = computed({
  get: () => props.modelValue ?? '',
  set: (value) => emit('update:modelValue', value)
})
</script>

<template>
  <textarea
    v-model="modelValue"
    :placeholder="placeholder"
    :disabled="disabled"
    :rows="rows ?? 3"
    :class="cn(
      'flex min-h-[80px] w-full rounded-lg border border-white/[0.1] bg-white/[0.04] px-3 py-2 text-sm text-white transition-colors duration-150 placeholder:text-white/40 hover:border-white/20 light:bg-white light:border-gray-200 light:text-gray-900 light:placeholder:text-gray-400 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500/30 focus-visible:border-emerald-500/60 disabled:cursor-not-allowed disabled:opacity-50 light:hover:border-gray-300 light:focus-visible:ring-emerald-500 light:focus-visible:border-emerald-500',
      props.class
    )"
  />
</template>
