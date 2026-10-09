<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader, ErrorState, StatCard } from '@/components/shared'
import { adminService, type AdminStats } from '@/services/api'
import { formatMoney } from '@/stores/wallet'
import { Line, Bar, shortDayLabel } from '@/lib/charts'
import { toast } from 'vue-sonner'
import { getErrorMessage } from '@/lib/api-utils'
import {
  Crown, Building2, Users, Phone, MessageSquare, MessageSquareReply, TrendingUp, ArrowDownToLine,
  RefreshCw, AlertTriangle
} from 'lucide-vue-next'

const { t } = useI18n()
const stats = ref<AdminStats | null>(null)
const isLoading = ref(true)
const error = ref(false)

async function load() {
  isLoading.value = true
  error.value = false
  try {
    const res = await adminService.stats()
    stats.value = res.data.data
  } catch (e) {
    error.value = true
    toast.error(getErrorMessage(e, t('owner.loadFailed')))
  } finally {
    isLoading.value = false
  }
}

onMounted(load)

const money = (v: number) => formatMoney(v, stats.value?.currency || 'INR')
const num = (v: number) => new Intl.NumberFormat().format(v)

const kpis = computed(() => {
  const s = stats.value
  if (!s) return []
  return [
    { label: t('owner.totalClients'), value: num(s.total_clients), sub: t('owner.newThisMonth', { n: s.new_clients_this_month }), icon: Building2 },
    { label: t('owner.activeClients'), value: num(s.active_clients), sub: t('owner.suspendedCount', { n: s.suspended_clients }), icon: Crown },
    { label: t('owner.revenueThisMonth'), value: money(s.spend_this_month), sub: t('owner.todayValue', { v: money(s.spend_today) }), icon: TrendingUp },
    { label: t('owner.rechargesThisMonth'), value: money(s.recharges_this_month), sub: t('owner.walletFloat', { v: money(s.total_wallet_balance) }), icon: ArrowDownToLine },
    { label: t('owner.messagesSent'), value: num(s.messages_sent_this_month), sub: t('owner.thisMonth'), icon: MessageSquare },
    { label: t('owner.messagesReceived'), value: num(s.messages_received_this_month), sub: t('owner.todayValue', { v: num(s.messages_today) }), icon: MessageSquareReply },
    { label: t('owner.whatsappNumbers'), value: num(s.total_numbers), sub: t('owner.contactsCount', { n: num(s.total_contacts) }, s.total_contacts), icon: Phone },
    { label: t('owner.totalUsers'), value: num(s.total_users), sub: t('owner.acrossClients'), icon: Users },
  ]
})

const labels = computed(() => (stats.value?.daily || []).map(d => shortDayLabel(d.date)))

const messagesChart = computed(() => ({
  labels: labels.value,
  datasets: [
    { label: t('owner.sent'), data: (stats.value?.daily || []).map(d => d.outgoing), borderColor: '#10b981', backgroundColor: 'rgba(16,185,129,0.08)', fill: true, tension: 0.35 },
    { label: t('owner.received'), data: (stats.value?.daily || []).map(d => d.incoming), borderColor: '#8a8f98', borderDash: [4, 4], tension: 0.35 },
  ]
}))

const revenueChart = computed(() => ({
  labels: labels.value,
  datasets: [
    { label: t('owner.revenue'), data: (stats.value?.daily || []).map(d => d.spend), backgroundColor: '#10b981' },
  ]
}))

const signupsChart = computed(() => ({
  labels: labels.value,
  datasets: [
    { label: t('owner.newClients'), data: (stats.value?.daily || []).map(d => d.signups), backgroundColor: '#10b981' },
  ]
}))

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: { legend: { display: true, position: 'top' as const } },
  scales: { y: { beginAtZero: true, ticks: { precision: 0 } }, x: { grid: { display: false } } }
}
// Single-series charts are already named by their card title
const barOptions = { ...chartOptions, plugins: { legend: { display: false } } }
const revenueOptions = computed(() => ({
  ...barOptions,
  plugins: { legend: { display: false }, tooltip: { callbacks: { label: (ctx: { parsed: { y: number | null } }) => money(ctx.parsed.y ?? 0) } } },
  scales: { ...chartOptions.scales, y: { beginAtZero: true, ticks: { callback: (v: string | number) => money(Number(v)) } } },
}))
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('owner.dashboardTitle')" :description="$t('owner.dashboardSubtitle')" :icon="Crown">
      <template #actions>
        <Button variant="outline" size="sm" :disabled="isLoading" @click="load"><RefreshCw :class="['h-4 w-4', isLoading && 'animate-spin']" />{{ $t('common.refresh') }}</Button>
        <RouterLink to="/admin/clients"><Button size="sm"><Building2 class="h-4 w-4" />{{ $t('owner.manageClients') }}</Button></RouterLink>
      </template>
    </PageHeader>

    <ErrorState v-if="error && !isLoading" :title="$t('common.loadErrorTitle')" :description="$t('common.loadErrorDescription')" :retry-label="$t('common.retryLoad')" class="flex-1" @retry="load" />

    <ScrollArea v-else class="flex-1">
      <div class="p-6 space-y-6">
        <div v-if="stats && !stats.billing_enabled" class="flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-300 light:text-amber-700">
          <AlertTriangle class="h-4 w-4 shrink-0" />{{ $t('owner.billingDisabled') }}
        </div>

        <!-- KPI cards -->
        <div class="grid gap-4 grid-cols-1 sm:grid-cols-2 xl:grid-cols-4">
          <template v-if="isLoading && !stats">
            <Skeleton v-for="i in 8" :key="i" class="h-28 rounded-xl" />
          </template>
          <StatCard v-for="k in kpis" v-else :key="k.label" :label="k.label" :value="k.value" :sub="k.sub" :icon="k.icon" />
        </div>

        <!-- Charts -->
        <div v-if="stats" class="grid gap-4 lg:grid-cols-2">
          <Card>
            <CardHeader><CardTitle class="text-base">{{ $t('owner.messagesLast30') }}</CardTitle></CardHeader>
            <CardContent><div class="h-64"><Line :data="messagesChart" :options="chartOptions" /></div></CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle class="text-base">{{ $t('owner.revenueLast30') }}</CardTitle></CardHeader>
            <CardContent><div class="h-64"><Bar :data="revenueChart" :options="revenueOptions" /></div></CardContent>
          </Card>
        </div>

        <div v-if="stats" class="grid gap-4 lg:grid-cols-3">
          <!-- Top clients -->
          <Card>
            <CardHeader>
              <CardTitle class="text-base">{{ $t('owner.topClients') }}</CardTitle>
              <CardDescription>{{ $t('owner.topClientsDesc') }}</CardDescription>
            </CardHeader>
            <CardContent>
              <p v-if="!stats.top_clients?.length" class="text-sm text-muted-foreground">{{ $t('owner.noSpendYet') }}</p>
              <ul v-else class="space-y-3">
                <li v-for="(c, i) in stats.top_clients" :key="c.id" class="flex items-center justify-between gap-3">
                  <RouterLink :to="`/admin/clients/${c.id}`" class="flex items-center gap-3 min-w-0 hover:underline">
                    <span class="h-6 w-6 shrink-0 rounded-full bg-white/[0.06] light:bg-gray-100 text-xs tabular-nums text-muted-foreground flex items-center justify-center">{{ i + 1 }}</span>
                    <span class="truncate text-sm font-medium text-foreground">{{ c.name }}</span>
                  </RouterLink>
                  <div class="text-right">
                    <div class="text-sm font-medium tabular-nums">{{ money(c.spend) }}</div>
                    <div class="text-xs text-muted-foreground">{{ $t('owner.nMessages', { n: num(c.messages) }, c.messages) }}</div>
                  </div>
                </li>
              </ul>
            </CardContent>
          </Card>

          <!-- Low balance -->
          <Card>
            <CardHeader>
              <CardTitle class="text-base">{{ $t('owner.lowBalanceClients') }}</CardTitle>
              <CardDescription>{{ $t('owner.lowBalanceClientsDesc') }}</CardDescription>
            </CardHeader>
            <CardContent>
              <p v-if="!stats.low_balance_clients?.length" class="text-sm text-muted-foreground">{{ $t('owner.allHealthy') }}</p>
              <ul v-else class="space-y-3">
                <li v-for="c in stats.low_balance_clients" :key="c.id" class="flex items-center justify-between gap-3">
                  <RouterLink :to="`/admin/clients/${c.id}`" class="truncate text-sm font-medium text-foreground hover:underline">{{ c.name }}</RouterLink>
                  <span :class="['text-sm font-medium tabular-nums', c.balance <= 0 ? 'text-red-400' : 'text-amber-400']">{{ money(c.balance) }}</span>
                </li>
              </ul>
            </CardContent>
          </Card>

          <!-- Signups -->
          <Card>
            <CardHeader>
              <CardTitle class="text-base">{{ $t('owner.newClientsLast30') }}</CardTitle>
            </CardHeader>
            <CardContent><div class="h-48"><Bar :data="signupsChart" :options="barOptions" /></div></CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>
  </div>
</template>
