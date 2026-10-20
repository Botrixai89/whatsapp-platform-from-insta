<script setup lang="ts">
import { computed } from 'vue'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
} from 'lucide-vue-next'
import { getPageNumbers } from '@/composables/usePagination'

const props = defineProps<{
  currentPage: number
  totalPages: number
  totalItems: number
  pageSize: number
  itemName?: string
  /** Offer a rows-per-page picker with these sizes */
  pageSizeOptions?: number[]
}>()

const emit = defineEmits<{
  'update:currentPage': [page: number]
  'update:pageSize': [size: number]
}>()

function onPageSize(v: unknown) {
  const n = Number(v)
  if (n > 0) emit('update:pageSize', n)
}

const paginationInfo = computed(() => {
  const start = (props.currentPage - 1) * props.pageSize + 1
  const end = Math.min(props.currentPage * props.pageSize, props.totalItems)
  return { start, end }
})

const pageNumbers = computed(() => getPageNumbers(props.currentPage, props.totalPages))

function goToPage(page: number | '...') {
  if (page === '...') return
  emit('update:currentPage', page)
}
</script>

<template>
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div class="flex items-center gap-3">
      <p class="text-sm text-muted-foreground">
        {{ $t('common.showingRange', { start: paginationInfo.start, end: paginationInfo.end, total: totalItems, items: itemName || $t('common.items') }) }}
      </p>
      <Select v-if="pageSizeOptions?.length" :model-value="String(pageSize)" @update:model-value="onPageSize">
        <SelectTrigger class="h-8 w-[118px] text-xs" :aria-label="$t('common.rowsPerPage')">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem v-for="n in pageSizeOptions" :key="n" :value="String(n)">{{ $t('common.perPage', { n }) }}</SelectItem>
        </SelectContent>
      </Select>
    </div>
    <div v-if="totalPages > 1" class="flex items-center gap-1">
      <Button
        variant="outline"
        size="icon"
        class="h-8 w-8"
        :disabled="currentPage === 1"
        @click="goToPage(1)"
      >
        <ChevronsLeft class="h-4 w-4" />
      </Button>
      <Button
        variant="outline"
        size="icon"
        class="h-8 w-8"
        :disabled="currentPage === 1"
        @click="goToPage(currentPage - 1)"
      >
        <ChevronLeft class="h-4 w-4" />
      </Button>
      <div class="flex items-center gap-1 mx-2">
        <template v-for="(page, index) in pageNumbers" :key="index">
          <Button
            v-if="page !== '...'"
            :variant="page === currentPage ? 'default' : 'outline'"
            size="icon"
            class="h-8 w-8"
            @click="goToPage(page)"
          >
            {{ page }}
          </Button>
          <span v-else class="px-1 text-muted-foreground">...</span>
        </template>
      </div>
      <Button
        variant="outline"
        size="icon"
        class="h-8 w-8"
        :disabled="currentPage === totalPages"
        @click="goToPage(currentPage + 1)"
      >
        <ChevronRight class="h-4 w-4" />
      </Button>
      <Button
        variant="outline"
        size="icon"
        class="h-8 w-8"
        :disabled="currentPage === totalPages"
        @click="goToPage(totalPages)"
      >
        <ChevronsRight class="h-4 w-4" />
      </Button>
    </div>
  </div>
</template>
