<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Progress } from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { campaignsService } from '@/services/api'
import { wsService } from '@/services/websocket'
import { toast } from 'vue-sonner'
import { PageHeader, DataTable, DeleteConfirmDialog, SearchInput, IconButton, ErrorState, DateRangePicker, type Column } from '@/components/shared'
import { getErrorMessage } from '@/lib/api-utils'
import {
  Plus,
  Pencil,
  Trash2,
  Megaphone,
  Play,
  Pause,
  CheckCircle,
  Clock,
  AlertCircle,
  CalendarX2,
  ListChecks,
  CalendarClock,
} from 'lucide-vue-next'
import { formatDateTime } from '@/lib/utils'
import { useSearchPagination } from '@/composables/useSearchPagination'
import { useDateRange } from '@/composables/useDateRange'
import CampaignLogsTab from './campaigns/CampaignLogsTab.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

interface Campaign {
  id: string
  name: string
  template_name: string
  template_id?: string
  whatsapp_account?: string
  status: 'draft' | 'scheduled' | 'running' | 'paused' | 'completed' | 'failed' | 'queued' | 'processing' | 'cancelled'
  total_recipients: number
  sent_count: number
  delivered_count: number
  read_count: number
  failed_count: number
  pending_count?: number
  replied_count?: number
  scheduled_at?: string
  started_at?: string
  completed_at?: string
  created_at: string
}

type CampaignTab = 'broadcasts' | 'scheduled' | 'logs'
const tab = ref<CampaignTab>(['scheduled', 'logs'].includes(route.query.tab as string) ? route.query.tab as CampaignTab : 'broadcasts')
watch(tab, (v) => {
  router.replace({ query: { ...route.query, tab: v === 'broadcasts' ? undefined : v } })
  if (v !== 'logs') resetAndFetch()
})
const isScheduledTab = computed(() => tab.value === 'scheduled')

const campaigns = ref<Campaign[]>([])
const isLoading = ref(true)

const columns = computed<Column<Campaign>[]>(() => isScheduledTab.value
  ? [
      { key: 'name', label: t('campaigns.campaign'), sortable: true },
      { key: 'scheduled_at', label: t('campaigns.scheduledForCol'), sortable: true },
      { key: 'total_recipients', label: t('campaigns.recipients'), align: 'center', sortable: true },
      { key: 'whatsapp_account', label: t('campaigns.sendFrom') },
      { key: 'actions', label: t('common.actions'), align: 'right' },
    ]
  : [
      { key: 'name', label: t('campaigns.campaign'), sortable: true },
      { key: 'sent_on', label: t('campaigns.sentOn'), sortKey: 'created_at', sortable: true },
      { key: 'total_recipients', label: t('campaigns.recipients'), align: 'center', sortable: true },
      { key: 'status', label: t('campaigns.status'), align: 'center', sortable: true },
      { key: 'processing', label: t('campaigns.processing'), align: 'center' },
      { key: 'delivered', label: t('campaigns.delivered'), align: 'center' },
      { key: 'seen', label: t('campaigns.seen'), align: 'center' },
      { key: 'replied', label: t('campaigns.replied'), align: 'center' },
      { key: 'failed', label: t('campaigns.failed'), align: 'center' },
      { key: 'actions', label: '', align: 'right' },
    ])

const sortKey = ref('created_at')
const sortDirection = ref<'asc' | 'desc'>('desc')
const { searchQuery, currentPage, totalItems, perPage, setPageSize, handlePageChange, resetAndFetch } = useSearchPagination({
  fetchFn: () => fetchCampaigns(),
})

// Filter state
const filterStatus = ref<string>('all')
const {
  selectedRange,
  customDateRange,
  isDatePickerOpen,
  dateRange,
  formatDateRangeDisplay,
  applyCustomRange: applyCustomRangeBase,
} = useDateRange({ storageKey: 'campaigns_list' })

const statusOptions = computed(() => [
  { value: 'all', label: t('campaigns.allStatuses') },
  { value: 'draft', label: t('campaigns.draft') },
  { value: 'queued', label: t('campaigns.queued') },
  { value: 'processing', label: t('campaigns.processing') },
  { value: 'completed', label: t('campaigns.completed') },
  { value: 'failed', label: t('campaigns.failed') },
  { value: 'cancelled', label: t('campaigns.cancelled') },
  { value: 'paused', label: t('campaigns.paused') },
])

// AlertDialog state
const deleteDialogOpen = ref(false)
const campaignToDelete = ref<Campaign | null>(null)
const isDeletingCampaign = ref(false)

// Error state
const error = ref<string | null>(null)

// WebSocket subscription for real-time stats updates
let unsubscribeCampaignStats: (() => void) | null = null

onMounted(async () => {
  if (tab.value !== 'logs') await fetchCampaigns()

  // Subscribe to campaign stats updates
  unsubscribeCampaignStats = wsService.onCampaignStatsUpdate((payload) => {
    const campaign = campaigns.value.find(c => c.id === payload.campaign_id)
    if (campaign) {
      campaign.sent_count = payload.sent_count
      campaign.delivered_count = payload.delivered_count
      campaign.read_count = payload.read_count
      campaign.failed_count = payload.failed_count
      if (campaign.pending_count != null) {
        campaign.pending_count = Math.max(0, campaign.total_recipients - payload.sent_count - payload.failed_count)
      }
      if (payload.status) {
        campaign.status = payload.status
      }
    }
  })
})

onUnmounted(() => {
  if (unsubscribeCampaignStats) {
    unsubscribeCampaignStats()
  }
})

async function fetchCampaigns() {
  if (tab.value === 'logs') return
  isLoading.value = true
  error.value = null
  try {
    const params: Record<string, string | number> = {
      page: currentPage.value,
      limit: perPage.value,
    }
    if (isScheduledTab.value) {
      params.status = 'scheduled'
    } else {
      const { from, to } = dateRange.value
      params.from = from
      params.to = to
      if (filterStatus.value && filterStatus.value !== 'all') {
        params.status = filterStatus.value
      }
    }
    if (searchQuery.value) {
      params.search = searchQuery.value
    }
    const response = await campaignsService.list(params)
    // API returns: { status: "success", data: { campaigns: [...], total: N } }
    const data = response.data.data || response.data
    campaigns.value = data.campaigns || []
    totalItems.value = data.total ?? campaigns.value.length
  } catch (err: any) {
    console.error('Failed to fetch campaigns:', err)
    error.value = getErrorMessage(err, t('campaigns.fetchFailed'))
    campaigns.value = []
    totalItems.value = 0
  } finally {
    isLoading.value = false
  }
}

function applyCustomRange() {
  applyCustomRangeBase()
  resetAndFetch()
}

// Watch for filter changes
watch([filterStatus, selectedRange], () => {
  if (selectedRange.value !== 'custom') {
    resetAndFetch()
  }
})

function openDeleteDialog(campaign: Campaign) {
  campaignToDelete.value = campaign
  deleteDialogOpen.value = true
}

async function confirmDeleteCampaign() {
  if (!campaignToDelete.value) return

  isDeletingCampaign.value = true
  try {
    await campaignsService.delete(campaignToDelete.value.id)
    toast.success(t('common.deletedSuccess', { resource: t('resources.Campaign') }))
    deleteDialogOpen.value = false
    campaignToDelete.value = null
    await fetchCampaigns()
  } catch (error: any) {
    toast.error(getErrorMessage(error, t('common.failedDelete', { resource: t('resources.campaign') })))
  } finally {
    isDeletingCampaign.value = false
  }
}

const unscheduling = ref<string | null>(null)
async function unschedule(campaign: Campaign) {
  unscheduling.value = campaign.id
  try {
    await campaignsService.schedule(campaign.id, null)
    toast.success(t('campaigns.unscheduled'))
    await fetchCampaigns()
  } catch (e) {
    toast.error(getErrorMessage(e, t('campaigns.scheduleFailed')))
  } finally {
    unscheduling.value = null
  }
}

function getStatusIcon(status: string) {
  switch (status) {
    case 'completed':
      return CheckCircle
    case 'running':
    case 'processing':
    case 'queued':
      return Play
    case 'paused':
      return Pause
    case 'scheduled':
      return Clock
    case 'failed':
    case 'cancelled':
      return AlertCircle
    default:
      return Megaphone
  }
}

function getStatusClass(status: string): string {
  switch (status) {
    case 'completed':
      return 'border-emerald-600/60 text-emerald-500 light:text-emerald-700'
    case 'running':
    case 'processing':
    case 'queued':
      return 'border-sky-600/60 text-sky-500 light:text-sky-700'
    case 'scheduled':
      return 'border-amber-600/60 text-amber-500 light:text-amber-700'
    case 'failed':
    case 'cancelled':
      return 'border-destructive text-destructive'
    default:
      return 'text-muted-foreground'
  }
}

function statusLabel(status: string) {
  const key = `campaigns.${status}`
  const label = t(key)
  return label === key ? status : label
}

function getProgressPercentage(campaign: Campaign): number {
  if (campaign.total_recipients === 0) return 0
  return Math.round(((campaign.sent_count + campaign.failed_count) / campaign.total_recipients) * 100)
}

const isLive = (c: Campaign) => ['running', 'processing', 'queued'].includes(c.status)
const notSentYet = (c: Campaign) => ['draft', 'scheduled'].includes(c.status)

// A read message was delivered too, even when the delivery receipt arrived late
const deliveredOf = (c: Campaign) => Math.max(c.delivered_count, c.read_count)

function pct(n: number, c: Campaign) {
  const base = c.sent_count || c.total_recipients
  if (!base || !n) return ''
  return `${Math.min(100, Math.round((n / base) * 100))}%`
}

function sentOn(c: Campaign) {
  return c.started_at || c.completed_at || ''
}
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader
      :title="$t('campaigns.title')"
      :subtitle="$t('campaigns.subtitle')"
      :icon="Megaphone"
    >
      <template #actions>
        <RouterLink to="/campaigns/new">
          <Button size="sm">
            <Plus class="h-4 w-4 mr-2" />
            {{ $t('campaigns.createCampaign') }}
          </Button>
        </RouterLink>
      </template>
    </PageHeader>

    <ScrollArea class="flex-1">
      <div class="p-6">
        <Tabs v-model="tab" class="space-y-6">
          <TabsList class="h-auto bg-white/[0.04] p-1 ring-1 ring-white/[0.08] light:bg-gray-100 light:ring-gray-200">
            <TabsTrigger value="broadcasts" class="px-4 py-1.5"><Megaphone class="mr-2 h-4 w-4" />{{ $t('campaigns.tabBroadcasts') }}</TabsTrigger>
            <TabsTrigger value="scheduled" class="px-4 py-1.5"><CalendarClock class="mr-2 h-4 w-4" />{{ $t('campaigns.tabScheduled') }}</TabsTrigger>
            <TabsTrigger value="logs" class="px-4 py-1.5"><ListChecks class="mr-2 h-4 w-4" />{{ $t('campaigns.tabLogs') }}</TabsTrigger>
          </TabsList>

          <TabsContent value="logs" class="mt-0">
            <CampaignLogsTab />
          </TabsContent>

          <TabsContent v-for="pane in ['broadcasts', 'scheduled']" :key="pane" :value="pane" class="mt-0">
            <Card v-if="tab === pane">
              <CardHeader>
                <div class="flex flex-wrap items-center gap-2">
                  <template v-if="!isScheduledTab">
                    <Select v-model="filterStatus">
                      <SelectTrigger class="w-[150px]" :aria-label="$t('campaigns.status')">
                        <SelectValue :placeholder="$t('campaigns.allStatuses')" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem v-for="opt in statusOptions" :key="opt.value" :value="opt.value">
                          {{ opt.label }}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                    <DateRangePicker
                      v-model:selected-range="selectedRange"
                      v-model:custom-date-range="customDateRange"
                      v-model:is-date-picker-open="isDatePickerOpen"
                      :format-date-range-display="formatDateRangeDisplay"
                      @apply-custom="applyCustomRange"
                    />
                  </template>
                  <p v-else class="mr-auto text-sm text-muted-foreground">{{ $t('campaigns.scheduledDesc') }}</p>
                  <SearchInput v-model="searchQuery" :placeholder="$t('campaigns.searchCampaigns') + '...'" class="w-full sm:w-56" />
                </div>
              </CardHeader>
              <CardContent>
                <ErrorState
                  v-if="error && !isLoading"
                  :title="$t('campaigns.fetchFailedTitle')"
                  :description="error"
                  :retry-label="$t('common.retry')"
                  @retry="fetchCampaigns"
                />
                <DataTable
                  v-else
                  :items="campaigns"
                  :columns="columns"
                  :is-loading="isLoading"
                  :empty-icon="isScheduledTab ? CalendarClock : Megaphone"
                  :empty-title="searchQuery ? $t('campaigns.noMatchingCampaigns') : isScheduledTab ? $t('campaigns.noScheduled') : $t('campaigns.noCampaignsYet')"
                  :empty-description="searchQuery ? $t('campaigns.noMatchingCampaignsDesc') : isScheduledTab ? $t('campaigns.noScheduledDesc') : $t('campaigns.noCampaignsYetDesc')"
                  v-model:sort-key="sortKey"
                  v-model:sort-direction="sortDirection"
                  server-pagination
                  :current-page="currentPage"
                  :total-items="totalItems"
                  :page-size="perPage"
                  :page-size-options="[20, 50, 100]"
                  :item-name="$t('resources.campaigns')"
                  @page-change="handlePageChange"
                  @update:page-size="setPageSize"
                >
                  <template #cell-name="{ item: campaign }">
                    <RouterLink :to="`/campaigns/${campaign.id}`" class="block max-w-[240px] text-inherit no-underline hover:opacity-80">
                      <span class="block truncate font-medium">{{ campaign.name }}</span>
                      <span v-if="campaign.template_name" class="block truncate text-xs text-muted-foreground">{{ campaign.template_name }}</span>
                    </RouterLink>
                  </template>
                  <template #cell-sent_on="{ item: campaign }">
                    <span v-if="sentOn(campaign)" class="whitespace-nowrap text-sm text-muted-foreground">{{ formatDateTime(sentOn(campaign)) }}</span>
                    <span v-else-if="campaign.status === 'scheduled' && campaign.scheduled_at" class="whitespace-nowrap text-sm text-amber-500 light:text-amber-700">
                      {{ $t('campaigns.scheduledShort', { date: formatDateTime(campaign.scheduled_at) }) }}
                    </span>
                    <span v-else class="whitespace-nowrap text-sm text-muted-foreground">{{ $t('campaigns.createdOn', { date: formatDateTime(campaign.created_at) }) }}</span>
                  </template>
                  <template #cell-scheduled_at="{ item: campaign }">
                    <span class="whitespace-nowrap text-sm">{{ campaign.scheduled_at ? formatDateTime(campaign.scheduled_at) : '—' }}</span>
                  </template>
                  <template #cell-whatsapp_account="{ item: campaign }">
                    <span class="text-sm text-muted-foreground">{{ campaign.whatsapp_account || '—' }}</span>
                  </template>
                  <template #cell-total_recipients="{ item: campaign }">
                    <span class="tabular-nums">{{ campaign.total_recipients }}</span>
                  </template>
                  <template #cell-status="{ item: campaign }">
                    <div class="flex flex-col items-center gap-1">
                      <Badge variant="outline" :class="[getStatusClass(campaign.status), 'text-xs capitalize']">
                        <component :is="getStatusIcon(campaign.status)" class="h-3 w-3 mr-1" />
                        {{ statusLabel(campaign.status) }}
                      </Badge>
                      <div v-if="isLive(campaign)" class="w-20">
                        <Progress :model-value="getProgressPercentage(campaign)" class="h-1" />
                      </div>
                    </div>
                  </template>
                  <template #cell-processing="{ item: campaign }">
                    <span class="tabular-nums" :class="{ 'text-muted-foreground': notSentYet(campaign) }">
                      {{ notSentYet(campaign) ? '—' : (campaign.pending_count ?? 0) }}
                    </span>
                  </template>
                  <template #cell-delivered="{ item: campaign }">
                    <div class="tabular-nums leading-tight">
                      {{ deliveredOf(campaign) }}
                      <span v-if="pct(deliveredOf(campaign), campaign)" class="block text-[11px] text-muted-foreground">{{ pct(deliveredOf(campaign), campaign) }}</span>
                    </div>
                  </template>
                  <template #cell-seen="{ item: campaign }">
                    <div class="tabular-nums leading-tight">
                      {{ campaign.read_count }}
                      <span v-if="pct(campaign.read_count, campaign)" class="block text-[11px] text-muted-foreground">{{ pct(campaign.read_count, campaign) }}</span>
                    </div>
                  </template>
                  <template #cell-replied="{ item: campaign }">
                    <div class="tabular-nums leading-tight">
                      {{ campaign.replied_count ?? 0 }}
                      <span v-if="pct(campaign.replied_count ?? 0, campaign)" class="block text-[11px] text-muted-foreground">{{ pct(campaign.replied_count ?? 0, campaign) }}</span>
                    </div>
                  </template>
                  <template #cell-failed="{ item: campaign }">
                    <span class="tabular-nums" :class="campaign.failed_count > 0 ? 'text-destructive' : 'text-muted-foreground'">{{ campaign.failed_count }}</span>
                  </template>
                  <template #cell-actions="{ item: campaign }">
                    <div class="flex items-center justify-end gap-1">
                      <IconButton
                        v-if="campaign.status === 'scheduled'"
                        :icon="CalendarX2"
                        :label="$t('campaigns.unschedule')"
                        :loading="unscheduling === campaign.id"
                        class="h-8 w-8"
                        @click="unschedule(campaign)"
                      />
                      <RouterLink :to="`/campaigns/${campaign.id}`"><IconButton :icon="Pencil" :label="$t('campaigns.editCampaign')" class="h-8 w-8" /></RouterLink>
                      <IconButton
                        :icon="Trash2"
                        :label="$t('campaigns.deleteCampaign')"
                        class="h-8 w-8 text-destructive"
                        :disabled="campaign.status === 'running' || campaign.status === 'processing'"
                        @click="openDeleteDialog(campaign)"
                      />
                    </div>
                  </template>
                  <template #empty-action>
                    <RouterLink v-if="!searchQuery" to="/campaigns/new">
                      <Button variant="outline" size="sm">
                        <Plus class="h-4 w-4 mr-2" />
                        {{ $t('campaigns.createCampaign') }}
                      </Button>
                    </RouterLink>
                  </template>
                </DataTable>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>
    </ScrollArea>

    <DeleteConfirmDialog
      v-model:open="deleteDialogOpen"
      :title="$t('campaigns.deleteCampaign')"
      :item-name="campaignToDelete?.name"
      :is-submitting="isDeletingCampaign"
      @confirm="confirmDeleteCampaign"
    />
  </div>
</template>
