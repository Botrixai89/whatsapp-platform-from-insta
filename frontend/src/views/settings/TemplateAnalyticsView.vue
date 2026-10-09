<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { PageHeader, DataTable, DateRangePicker, SearchInput, ErrorState, StatCard, type Column } from '@/components/shared'
import TemplateStatusCards from './templates/TemplateStatusCards.vue'
import { api, templateAIService, type TemplateAnalytics, type TemplateUsageRow } from '@/services/api'
import { useDateRange } from '@/composables/useDateRange'
import { formatMoney } from '@/stores/wallet'
import { formatDate } from '@/lib/utils'
import { Line, shortDayLabel } from '@/lib/charts'
import { getErrorMessage } from '@/lib/api-utils'
import { toast } from 'vue-sonner'
import { BarChart3, Send, CheckCheck, Eye, XCircle, Wallet } from 'lucide-vue-next'

const { t } = useI18n()
const router = useRouter()

const data = ref<TemplateAnalytics | null>(null)
const isLoading = ref(true)
const error = ref(false)
const accounts = ref<Array<{ id: string; name: string }>>([])
const account = ref('all')
const search = ref('')
const sortKey = ref('sent')
const sortDirection = ref<'asc' | 'desc'>('desc')

const {
  selectedRange, customDateRange, isDatePickerOpen,
  dateRange, formatDateRangeDisplay, applyCustomRange: baseApplyCustomRange,
} = useDateRange({ defaultPreset: '30days', storageKey: 'template_analytics' })

async function load() {
  isLoading.value = true
  error.value = false
  try {
    const { from, to } = dateRange.value
    const res = await templateAIService.analytics({ from, to, account: account.value === 'all' ? undefined : account.value })
    data.value = res.data.data
  } catch (e) {
    error.value = true
    toast.error(getErrorMessage(e, t('templateAI.analyticsLoadFailed')))
  } finally {
    isLoading.value = false
  }
}

function applyCustomRange() {
  baseApplyCustomRange()
  load()
}
watch(selectedRange, (v) => { if (v !== 'custom') load() })

onMounted(async () => {
  try {
    accounts.value = (await api.get('/accounts')).data.data?.accounts || []
  } catch {
    accounts.value = []
  }
  load()
})

const currency = computed(() => data.value?.currency || 'INR')
const pct = (n: number, d: number) => (d > 0 ? `${((n / d) * 100).toFixed(1)}%` : '—')
const num = (v: number) => new Intl.NumberFormat().format(v)

const kpis = computed(() => {
  const tot = data.value?.totals
  if (!tot) return []
  return [
    { label: t('templateAI.kpiSent'), value: num(tot.sent), sub: '', icon: Send, tone: 'default' as const },
    { label: t('templateAI.kpiDelivered'), value: num(tot.delivered), sub: tot.sent ? t('templateAI.rateOfSent', { v: pct(tot.delivered, tot.sent) }) : '', icon: CheckCheck, tone: 'default' as const },
    { label: t('templateAI.kpiRead'), value: num(tot.read), sub: tot.delivered ? t('templateAI.rateOfDelivered', { v: pct(tot.read, tot.delivered) }) : '', icon: Eye, tone: 'default' as const },
    { label: t('templateAI.kpiFailed'), value: num(tot.failed), sub: tot.sent ? t('templateAI.rateOfSent', { v: pct(tot.failed, tot.sent) }) : '', icon: XCircle, tone: tot.failed ? 'negative' as const : 'default' as const },
    { label: t('templateAI.kpiSpend'), value: formatMoney(tot.spend, currency.value), sub: '', icon: Wallet, tone: 'default' as const },
  ]
})

const chartData = computed(() => {
  const daily = data.value?.daily || []
  return {
    labels: daily.map(d => shortDayLabel(d.date)),
    datasets: [
      { label: t('templateAI.kpiSent'), data: daily.map(d => d.sent), borderColor: '#10b981', backgroundColor: 'rgba(16,185,129,0.08)', fill: true, tension: 0.35 },
      { label: t('templateAI.kpiDelivered'), data: daily.map(d => d.delivered), borderColor: '#38bdf8', tension: 0.35 },
      { label: t('templateAI.kpiRead'), data: daily.map(d => d.read), borderColor: '#a78bfa', tension: 0.35 },
    ],
  }
})
const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: { legend: { display: true, position: 'top' as const } },
  scales: { y: { beginAtZero: true, ticks: { precision: 0 } }, x: { grid: { display: false } } },
}
const hasDailyData = computed(() => (data.value?.daily || []).some(d => d.sent > 0))

const rows = computed(() => {
  const q = search.value.trim().toLowerCase()
  const list = data.value?.templates || []
  return (q ? list.filter(r => r.name.toLowerCase().includes(q) || r.display_name?.toLowerCase().includes(q)) : list)
    .map(r => ({ ...r, delivery_rate: r.sent ? r.delivered / r.sent : 0, read_rate: r.delivered ? r.read / r.delivered : 0 }))
})

const columns = computed<Column<TemplateUsageRow>[]>(() => [
  { key: 'name', label: t('templateAI.colTemplate'), sortable: true },
  { key: 'category', label: t('templates.category', 'Category'), sortable: true },
  { key: 'status', label: t('common.status'), sortable: true },
  { key: 'sent', label: t('templateAI.kpiSent'), sortable: true, align: 'right' },
  { key: 'delivery_rate', label: t('templateAI.colDelivery'), sortable: true, align: 'right' },
  { key: 'read_rate', label: t('templateAI.colRead'), sortable: true, align: 'right' },
  { key: 'failed', label: t('templateAI.kpiFailed'), sortable: true, align: 'right' },
  { key: 'spend', label: t('templateAI.kpiSpend'), sortable: true, align: 'right' },
  { key: 'last_sent_at', label: t('templateAI.colLastSent'), sortable: true },
])

const categoryLabel = (s: string) => (s ? s.charAt(0).toUpperCase() + s.slice(1).toLowerCase() : '—')
const statusVariant = (s: string) => ({ APPROVED: 'success', PENDING: 'warning', REJECTED: 'destructive', PAUSED: 'warning', DRAFT: 'secondary' } as Record<string, any>)[s?.toUpperCase()] || 'secondary'
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('templateAI.analyticsTitle')" :description="$t('templateAI.analyticsSubtitle')" :icon="BarChart3" back-link="/templates">
      <template #actions>
          <Select v-model="account" @update:model-value="load">
            <SelectTrigger class="w-44"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{{ $t('templateAI.allNumbers') }}</SelectItem>
              <SelectItem v-for="a in accounts" :key="a.id" :value="a.name">{{ a.name }}</SelectItem>
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
    </PageHeader>

    <ErrorState v-if="error && !isLoading" :title="$t('common.loadErrorTitle')" :description="$t('common.loadErrorDescription')" :retry-label="$t('common.retryLoad')" class="flex-1" @retry="load" />

    <ScrollArea v-else class="flex-1">
      <div class="p-6 space-y-6">
        <TemplateStatusCards :counts="data?.status_counts || null" />

        <div class="grid gap-4 grid-cols-2 lg:grid-cols-5">
          <StatCard v-for="k in kpis" :key="k.label" :label="k.label" :value="k.value" :sub="k.sub" :icon="k.icon" :sub-tone="k.tone" />
        </div>

        <Card>
          <CardHeader><CardTitle class="text-base">{{ $t('templateAI.dailySends') }}</CardTitle></CardHeader>
          <CardContent>
            <div v-if="hasDailyData" class="h-64"><Line :data="chartData" :options="chartOptions" /></div>
            <div v-else class="h-64 flex flex-col items-center justify-center text-center">
              <BarChart3 class="h-5 w-5 text-white/30 light:text-gray-400 mb-2" />
              <p class="text-sm font-medium">{{ $t('templateAI.noSendsTitle') }}</p>
              <p class="text-sm text-muted-foreground">{{ $t('templateAI.noSendsDesc') }}</p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div class="flex items-center justify-between flex-wrap gap-4">
              <div>
                <CardTitle class="text-base">{{ $t('templateAI.perTemplate') }}</CardTitle>
                <CardDescription>{{ $t('templateAI.perTemplateDesc') }}</CardDescription>
              </div>
              <SearchInput v-model="search" :placeholder="$t('templates.searchTemplates', 'Search templates') + '...'" class="w-64" />
            </div>
          </CardHeader>
          <CardContent>
            <DataTable
              :items="rows"
              :columns="columns"
              :is-loading="isLoading"
              :empty-icon="BarChart3"
              :empty-title="$t('templateAI.noTemplateData')"
              v-model:sort-key="sortKey"
              v-model:sort-direction="sortDirection"
            >
              <template #cell-name="{ item }">
                <button class="text-left hover:underline" @click="router.push(`/templates/${item.id}`)">
                  <div class="font-medium">{{ item.display_name || item.name }}</div>
                  <div class="text-xs text-muted-foreground font-mono">{{ item.name }} · {{ item.language }}</div>
                </button>
              </template>
              <template #cell-category="{ item }"><span class="text-xs text-muted-foreground">{{ categoryLabel(item.category) }}</span></template>
              <template #cell-status="{ item }"><Badge :variant="statusVariant(item.status)">{{ categoryLabel(item.status) }}</Badge></template>
              <template #cell-sent="{ item }"><span class="tabular-nums">{{ num(item.sent) }}</span></template>
              <template #cell-delivery_rate="{ item }"><span class="tabular-nums">{{ pct(item.delivered, item.sent) }}</span></template>
              <template #cell-read_rate="{ item }"><span class="tabular-nums">{{ pct(item.read, item.delivered) }}</span></template>
              <template #cell-failed="{ item }"><span :class="['tabular-nums', item.failed ? 'text-red-400' : '']">{{ num(item.failed) }}</span></template>
              <template #cell-spend="{ item }"><span class="tabular-nums">{{ formatMoney(item.spend, currency) }}</span></template>
              <template #cell-last_sent_at="{ item }"><span class="text-muted-foreground">{{ item.last_sent_at ? formatDate(item.last_sent_at) : '—' }}</span></template>
            </DataTable>
          </CardContent>
        </Card>
      </div>
    </ScrollArea>
  </div>
</template>
