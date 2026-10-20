<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { SearchInput } from '@/components/shared'
import WhatsAppTemplatePreview from './WhatsAppTemplatePreview.vue'
import { galleryTemplates, galleryIndustries, galleryToDraft, type GalleryIndustry } from './templateGallery'
import { LayoutTemplate } from 'lucide-vue-next'

/** Ready-made templates by industry; picking one opens the editor pre-filled. */
defineProps<{ canWrite: boolean }>()
const router = useRouter()
const { t } = useI18n()

const industry = ref<GalleryIndustry | 'all'>('all')
const search = ref('')

const counts = computed(() => {
  const c: Record<string, number> = {}
  for (const g of galleryTemplates) c[g.industry] = (c[g.industry] || 0) + 1
  return c
})

const items = computed(() => {
  const q = search.value.trim().toLowerCase()
  return galleryTemplates.filter(g =>
    (industry.value === 'all' || g.industry === industry.value) &&
    (!q || g.title.toLowerCase().includes(q) || g.body.toLowerCase().includes(q)),
  ).map(g => ({ ...g, draft: galleryToDraft(g) }))
})

function useTemplate(id: string) {
  router.push({ path: '/templates/new', query: { gallery: id } })
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-center gap-3">
      <div class="wa-chips flex min-w-0 flex-1 gap-2 overflow-x-auto pr-6">
        <button type="button" class="wa-chip shrink-0" :class="{ 'is-active': industry === 'all' }" @click="industry = 'all'">
          {{ $t('templateGallery.all') }} <span class="opacity-60">{{ galleryTemplates.length }}</span>
        </button>
        <button
          v-for="ind in galleryIndustries" :key="ind" type="button"
          class="wa-chip shrink-0" :class="{ 'is-active': industry === ind }" @click="industry = ind"
        >
          {{ $t(`templateGallery.industries.${ind}`) }} <span class="opacity-60">{{ counts[ind] || 0 }}</span>
        </button>
      </div>
      <SearchInput v-model="search" :placeholder="$t('templateGallery.search')" class="w-full sm:w-60" />
    </div>

    <div v-if="items.length" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
      <div
        v-for="g in items" :key="g.id"
        class="flex flex-col overflow-hidden rounded-xl border border-white/[0.08] bg-white/[0.02] light:border-gray-200 light:bg-white"
      >
        <div class="flex items-center justify-between gap-2 px-4 py-3">
          <div class="min-w-0">
            <p class="truncate text-sm font-medium">{{ g.title }}</p>
            <p class="text-xs text-muted-foreground">{{ $t(`templateGallery.industries.${g.industry}`) }}</p>
          </div>
          <Badge variant="outline" class="shrink-0 text-[10px]">{{ g.category }}</Badge>
        </div>
        <WhatsAppTemplatePreview :draft="g.draft" class="flex-1 rounded-none" />
        <div class="px-4 py-3">
          <Button size="sm" class="w-full" :disabled="!canWrite" @click="useTemplate(g.id)">
            {{ $t('templateGallery.useTemplate') }}
          </Button>
        </div>
      </div>
    </div>
    <div v-else class="flex flex-col items-center py-16 text-center text-muted-foreground">
      <LayoutTemplate class="mb-3 h-8 w-8 opacity-50" />
      <p class="text-sm">{{ t('templateGallery.noMatch') }}</p>
    </div>
  </div>
</template>
