<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader, ErrorState } from '@/components/shared'
import { adminService, type AdminStats } from '@/services/api'
import { formatMoney } from '@/stores/wallet'
import { Line, Bar } from '@/lib/charts'
import { toast } from 'vue-sonner'
import { getErrorMessage } from '@/lib/api-utils'
import {
  Crown, Building2, Users, Phone, MessageSquare, Wallet, TrendingUp, ArrowDownToLine,
  UserPlus, RefreshCw, AlertTriangle
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
    { label: t('owner.totalClients'), value: num(s.total_clients), sub: t('owner.newThisMonth', { n: s.new_clients_this_month }), icon: Building2, color: 'from-violet-500 to-purple-600' },
    { label: t('owner.activeClients'), value: num(s.active_clients), sub: t('owner.suspendedCount', { n: s.suspended_clients }), icon: Crown, color: 'from-emerald-500 to-green-600' },
    { label: t('owner.revenueThisMonth'), value: money(s.spend_this_month), sub: t('owner.todayValue', { v: money(s.spend_today) }), icon: TrendingUp, color: 'from-amber-500 to-orange-600' },
    { label: t('owner.rechargesThisMonth'), value: money(s.recharges_this_month), sub: t('owner.walletFloat', { v: money(s.total_wallet_balance) }), icon: ArrowDownToLine, color: 'from-sky-500 to-blue-600' },
    { label: t('owner.messagesSent'), value: num(s.messages_sent_this_month), sub: t('owner.thisMonth'), icon: MessageSquare, color: 'from-teal-500 to-cyan-600' },
    { label: t('owner.messagesReceived'), value: num(s.messages_received_this_month), sub: t('owner.todayValue', { v: num(s.messages_today) }), icon: MessageSquare, color: 'from-indigo-500 to-blue-600' },
    { label: t('owner.whatsappNumbers'), value: num(s.total_numbers), sub: t('owner.contactsCount', { n: num(s.total_contacts) }), icon: Phone, color: 'from-green-500 to-emerald-600' },
    { label: t('owner.totalUsers'), value: num(s.total_users), sub: t('owner.acrossClients'), icon: Users, color: 'from-pink-500 to-rose-600' },
  ]
})

const labels = computed(() => (stats.value?.daily || []).map(d => d.date.slice(5)))

const messagesChart = computed(() => ({
  labels: labels.value,
  datasets: [
    { label: t('owner.sent'), data: (stats.value?.daily || []).map(d => d.outgoing), borderColor: '#10b981', backgroundColor: 'rgba(16,185,129,0.15)', fill: true, tension: 0.3 },
    { label: t('owner.received'), data: (stats.value?.daily || []).map(d => d.incoming), borderColor: '#6366f1', backgroundColor: 'rgba(99,102,241,0.10)', fill: true, tension: 0.3 },
  ]
}))

const revenueChart = computed(() => ({
  labels: labels.value,
  datasets: [
    { label: t('owner.revenue'), data: (stats.value?.daily || []).map(d => d.spend), backgroundColor: '#f59e0b', borderRadius: 4 },
  ]
}))

const signupsChart = computed(() => ({
  labels: labels.value,
  datasets: [
    { label: t('owner.newClients'), data: (stats.value?.daily || []).map(d => d.signups), backgroundColor: '#8b5cf6', borderRadius: 4 },
  ]
}))

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: { legend: { display: true, position: 'top' as const } },
  scales: { y: { beginAtZero: true } }
}
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('owner.dashboardTitle')" :description="$t('owner.dashboardSubtitle')" :icon="Crown" icon-gradient="bg-gradient-to-br from-amber-500 to-orange-600 shadow-amber-500/20">
      <template #actions>
        <div class="flex gap-2">
          <Button variant="outline" size="sm" :disabled="isLoading" @click="load"><RefreshCw class="h-4 w-4 mr-2" />{{ $t('common.refresh') }}</Button>
          <RouterLink to="/admin/clients"><Button size="sm"><UserPlus class="h-4 w-4 mr-2" />{{ $t('owner.manageClients') }}</Button></RouterLink>
        </div>
      </template>
    </PageHeader>

    <ErrorState v-if="error && !isLoading" :title="$t('common.loadErrorTitle')" :description="$t('common.loadErrorDescription')" :retry-label="$t('common.retryLoad')" class="flex-1" @retry="load" />

    <ScrollArea v-else class="flex-1">
      <div class="p-6 max-w-7xl mx-auto space-y-6">
        <div v-if="stats && !stats.billing_enabled" class="flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-300 light:text-amber-700">
          <AlertTriangle class="h-4 w-4 shrink-0" />{{ $t('owner.billingDisabled') }}
        </div>

        <!-- KPI cards -->
        <div class="grid gap-4 grid-cols-1 sm:grid-cols-2 xl:grid-cols-4">
          <template v-if="isLoading && !stats">
            <Skeleton v-for="i in 8" :key="i" class="h-28 rounded-xl" />
          </template>
          <Card v-for="k in kpis" v-else :key="k.label">
            <CardContent class="p-5">
              <div class="flex items-start justify-between">
                <div class="min-w-0">
                  <p class="text-xs font-medium text-white/50 light:text-gray-500">{{ k.label }}</p>
                  <p class="mt-1 text-2xl font-semibold tabular-nums text-white light:text-gray-900 truncate">{{ k.value }}</p>
                  <p class="mt-1 text-xs text-white/40 light:text-gray-400">{{ k.sub }}</p>
                </div>
                <div :class="['h-9 w-9 shrink-0 rounded-lg flex items-center justify-center bg-gradient-to-br shadow-lg', k.color]">
                  <component :is="k.icon" class="h-4 w-4 text-white" />
                </div>
              </div>
            </CardContent>
          </Card>
        </div>

        <!-- Charts -->
        <div v-if="stats" class="grid gap-4 lg:grid-cols-2">
          <Card>
            <CardHeader><CardTitle class="text-base">{{ $t('owner.messagesLast30') }}</CardTitle></CardHeader>
            <CardContent><div class="h-64"><Line :data="messagesChart" :options="chartOptions" /></div></CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle class="text-base">{{ $t('owner.revenueLast30') }}</CardTitle></CardHeader>
            <CardContent><div class="h-64"><Bar :data="revenueChart" :options="chartOptions" /></div></CardContent>
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
                    <span class="h-6 w-6 shrink-0 rounded-full bg-white/[0.08] light:bg-gray-100 text-xs flex items-center justify-center">{{ i + 1 }}</span>
                    <span class="truncate text-sm">{{ c.name }}</span>
                  </RouterLink>
                  <div class="text-right">
                    <div class="text-sm font-medium tabular-nums">{{ money(c.spend) }}</div>
                    <div class="text-xs text-muted-foreground">{{ $t('owner.nMessages', { n: num(c.messages) }) }}</div>
                  </div>
                </li>
              </ul>
            </CardContent>
          </Card>

          <!-- Low balance -->
          <Card>
            <CardHeader>
              <CardTitle class="text-base flex items-center gap-2"><Wallet class="h-4 w-4" />{{ $t('owner.lowBalanceClients') }}</CardTitle>
              <CardDescription>{{ $t('owner.lowBalanceClientsDesc') }}</CardDescription>
            </CardHeader>
            <CardContent>
              <p v-if="!stats.low_balance_clients?.length" class="text-sm text-muted-foreground">{{ $t('owner.allHealthy') }}</p>
              <ul v-else class="space-y-3">
                <li v-for="c in stats.low_balance_clients" :key="c.id" class="flex items-center justify-between gap-3">
                  <RouterLink :to="`/admin/clients/${c.id}`" class="truncate text-sm hover:underline">{{ c.name }}</RouterLink>
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
            <CardContent><div class="h-48"><Bar :data="signupsChart" :options="{ ...chartOptions, plugins: { legend: { display: false } } }" /></div></CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>
  </div>
</template>
