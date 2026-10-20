<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { DataTable, SearchInput, ErrorState, type Column } from '@/components/shared'
import { campaignsService, type CampaignLog } from '@/services/api'
import { useSearchPagination } from '@/composables/useSearchPagination'
import { getErrorMessage } from '@/lib/api-utils'
import { formatDateTime } from '@/lib/utils'
import { toast } from 'vue-sonner'
import { Download, ListChecks, Loader2 } from 'lucide-vue-next'

/** Every broadcast message across campaigns, with its delivery status. */
const { t } = useI18n()

const logs = ref<CampaignLog[]>([])
const isLoading = ref(true)
const error = ref<string | null>(null)
const status = ref('all')
const period = ref('30')
const isExporting = ref(false)

const { searchQuery, currentPage, totalItems, perPage, setPageSize, handlePageChange, resetAndFetch } = useSearchPagination({
  pageSize: 50,
  fetchFn: () => fetchLogs(),
})

const statusOptions = ['pending', 'sent', 'delivered', 'read', 'failed'] as const
const periodOptions = ['1', '7', '30', '90', 'all']

const columns = computed<Column<CampaignLog>[]>(() => [
  { key: 'recipient', label: t('campaigns.logRecipient') },
  { key: 'campaign_name', label: t('campaigns.campaign') },
  { key: 'status', label: t('campaigns.status') },
  { key: 'sent_at', label: t('campaigns.logSentAt') },
  { key: 'delivered_at', label: t('campaigns.delivered') },
  { key: 'read_at', label: t('campaigns.seen') },
  { key: 'error_message', label: t('campaigns.logError') },
])

function filters() {
  const params: Record<string, string> = {}
  if (status.value !== 'all') params.status = status.value
  if (searchQuery.value) params.search = searchQuery.value
  if (period.value !== 'all') {
    const d = new Date()
    d.setDate(d.getDate() - (Number(period.value) - 1))
    params.from = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  }
  return params
}

async function fetchLogs() {
  isLoading.value = true
  error.value = null
  try {
    const res = await campaignsService.logs({ ...filters(), page: currentPage.value, limit: perPage.value })
    logs.value = res.data.data.logs || []
    totalItems.value = res.data.data.total ?? logs.value.length
  } catch (e) {
    error.value = getErrorMessage(e, t('campaigns.logsFailed'))
    logs.value = []
  } finally {
    isLoading.value = false
  }
}

async function exportCsv() {
  isExporting.value = true
  try {
    const res = await campaignsService.exportLogs(filters())
    const url = URL.createObjectURL(res.data)
    const a = document.createElement('a')
    a.href = url
    a.download = `broadcast-logs-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast.error(getErrorMessage(e, t('campaigns.exportFailed')))
  } finally {
    isExporting.value = false
  }
}

watch([status, period], () => resetAndFetch())
onMounted(fetchLogs)

function statusClass(s: string) {
  switch (s) {
    case 'read': return 'border-sky-600/60 text-sky-500 light:text-sky-700'
    case 'delivered': return 'border-emerald-600/60 text-emerald-500 light:text-emerald-700'
    case 'sent': return 'text-foreground'
    case 'failed': return 'border-destructive text-destructive'
    default: return 'text-muted-foreground'
  }
}
</script>

<template>
  <Card>
    <CardHeader>
      <div class="flex flex-wrap items-center gap-2">
        <Select v-model="status">
          <SelectTrigger class="w-[150px]" :aria-label="$t('campaigns.status')"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{{ $t('campaigns.allStatuses') }}</SelectItem>
            <SelectItem v-for="s in statusOptions" :key="s" :value="s">{{ $t(`campaigns.logStatus.${s}`) }}</SelectItem>
          </SelectContent>
        </Select>
        <Select v-model="period">
          <SelectTrigger class="w-[150px]" :aria-label="$t('campaigns.logPeriod')"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem v-for="p in periodOptions" :key="p" :value="p">
              {{ p === 'all' ? $t('campaigns.periodAll') : p === '1' ? $t('campaigns.periodToday') : $t('campaigns.periodDays', { n: p }) }}
            </SelectItem>
          </SelectContent>
        </Select>
        <SearchInput v-model="searchQuery" :placeholder="$t('campaigns.searchLogs')" class="w-full sm:w-64" />
        <Button variant="outline" size="sm" class="sm:ml-auto" :disabled="isExporting || !totalItems" @click="exportCsv">
          <Loader2 v-if="isExporting" class="h-4 w-4 animate-spin" />
          <Download v-else class="h-4 w-4" />
          {{ $t('campaigns.exportCsv') }}
        </Button>
      </div>
    </CardHeader>
    <CardContent>
      <ErrorState v-if="error && !isLoading" :title="$t('common.loadErrorTitle')" :description="error" :retry-label="$t('common.retry')" @retry="fetchLogs" />
      <DataTable
        v-else
        :items="logs"
        :columns="columns"
        :is-loading="isLoading"
        :empty-icon="ListChecks"
        :empty-title="$t('campaigns.noLogs')"
        :empty-description="$t('campaigns.noLogsDesc')"
        server-pagination
        :current-page="currentPage"
        :total-items="totalItems"
        :page-size="perPage"
        :page-size-options="[25, 50, 100]"
        :item-name="$t('campaigns.messages')"
        @page-change="handlePageChange"
        @update:page-size="setPageSize"
      >
        <template #cell-recipient="{ item }">
          <div class="leading-tight">
            <span class="block font-medium tabular-nums">{{ item.phone_number }}</span>
            <span v-if="item.recipient_name" class="block text-xs text-muted-foreground">{{ item.recipient_name }}</span>
          </div>
        </template>
        <template #cell-campaign_name="{ item }">
          <RouterLink :to="`/campaigns/${item.campaign_id}`" class="block max-w-[200px] truncate text-sm text-inherit no-underline hover:underline">{{ item.campaign_name }}</RouterLink>
        </template>
        <template #cell-status="{ item }">
          <Badge variant="outline" :class="[statusClass(item.status), 'text-xs']">{{ $t(`campaigns.logStatus.${item.status}`) }}</Badge>
        </template>
        <template #cell-sent_at="{ item }">
          <span class="whitespace-nowrap text-sm text-muted-foreground">{{ item.sent_at ? formatDateTime(item.sent_at) : '—' }}</span>
        </template>
        <template #cell-delivered_at="{ item }">
          <span class="whitespace-nowrap text-sm text-muted-foreground">{{ item.delivered_at ? formatDateTime(item.delivered_at) : '—' }}</span>
        </template>
        <template #cell-read_at="{ item }">
          <span class="whitespace-nowrap text-sm text-muted-foreground">{{ item.read_at ? formatDateTime(item.read_at) : '—' }}</span>
        </template>
        <template #cell-error_message="{ item }">
          <span v-if="item.error_message" class="block max-w-[260px] truncate text-xs text-destructive" :title="item.error_message">{{ item.error_message }}</span>
          <span v-else class="text-muted-foreground">—</span>
        </template>
      </DataTable>
    </CardContent>
  </Card>
</template>
