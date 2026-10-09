<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Card } from '@/components/ui/card'

/** Row of template counts by Meta review status. */
const props = defineProps<{ counts: Record<string, number> | null }>()
const { t } = useI18n()

const cards = computed(() => [
  { key: 'total', label: t('templateAI.statusTotal'), dot: '' },
  { key: 'approved', label: t('templateAI.statusApproved'), dot: 'bg-emerald-500' },
  { key: 'pending', label: t('templateAI.statusPending'), dot: 'bg-amber-500' },
  { key: 'rejected', label: t('templateAI.statusRejected'), dot: 'bg-red-500' },
  { key: 'paused', label: t('templateAI.statusPaused'), dot: 'bg-orange-400' },
  { key: 'draft', label: t('templateAI.statusDraft'), dot: 'bg-slate-400' },
])
</script>

<template>
  <!-- 1px gaps over a border-coloured backdrop draw the dividers at every breakpoint -->
  <Card class="grid grid-cols-2 sm:grid-cols-3 xl:grid-cols-6 gap-px overflow-hidden bg-white/[0.08] light:bg-gray-200">
    <div v-for="c in cards" :key="c.key" class="min-w-0 bg-[#0f0f10] px-5 py-4 light:bg-white">
      <p class="flex items-center gap-2 text-[13px] font-medium text-white/60 light:text-gray-600">
        <span v-if="c.dot" :class="['h-2 w-2 shrink-0 rounded-full', c.dot]" />
        <span class="truncate">{{ c.label }}</span>
      </p>
      <p class="mt-1.5 text-2xl font-semibold tracking-tight tabular-nums">{{ props.counts ? (props.counts[c.key] || 0) : '—' }}</p>
    </div>
  </Card>
</template>
