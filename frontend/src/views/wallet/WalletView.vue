<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { Button } from '@/components/ui/button'
import { PageHeader, DataTable, type Column } from '@/components/shared'
import TransactionsTable from '@/views/admin/TransactionsTable.vue'
import { walletService, type MessageRate } from '@/services/api'
import { useWalletStore, formatMoney } from '@/stores/wallet'
import { formatDate } from '@/lib/utils'
import { Wallet, AlertTriangle, Ban, RefreshCw, Info } from 'lucide-vue-next'

const { t } = useI18n()
const walletStore = useWalletStore()
const rates = ref<MessageRate[]>([])
const txTable = ref<InstanceType<typeof TransactionsTable> | null>(null)

const summary = computed(() => walletStore.summary)
const money = (v: number) => formatMoney(v, walletStore.currency)

onMounted(async () => {
  walletStore.fetch()
  try {
    rates.value = (await walletService.rates()).data.data.rates || []
  } catch {
    rates.value = []
  }
})

function refresh() {
  walletStore.fetch()
  txTable.value?.reload()
}

const usageRows = computed(() => {
  const u = summary.value?.usage
  if (!u) return []
  return [
    { label: t('owner.users'), used: u.users, limit: u.plan?.max_users || 0 },
    { label: t('owner.whatsappNumbers'), used: u.accounts, limit: u.plan?.max_accounts || 0 },
    { label: t('owner.messagesMonth'), used: u.messages_this_month, limit: u.plan?.max_monthly_messages || 0 },
  ]
})

const rateColumns = computed<Column<MessageRate>[]>(() => [
  { key: 'country', label: t('owner.country') },
  { key: 'category', label: t('owner.category') },
  { key: 'price', label: t('owner.pricePerMessage'), align: 'right' },
])
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('wallet.title')" :description="$t('wallet.subtitle')" :icon="Wallet" icon-gradient="bg-gradient-to-br from-emerald-500 to-green-600 shadow-emerald-500/20">
      <template #actions>
        <Button variant="outline" size="sm" @click="refresh"><RefreshCw class="h-4 w-4 mr-2" />{{ $t('common.refresh') }}</Button>
      </template>
    </PageHeader>

    <ScrollArea class="flex-1">
      <div class="p-6 max-w-6xl mx-auto space-y-6">
        <div v-if="summary?.status === 'suspended'" class="flex items-center gap-2 rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300 light:text-red-700">
          <Ban class="h-4 w-4 shrink-0" />{{ $t('wallet.suspendedBanner') }}<template v-if="summary.suspended_reason"> — {{ summary.suspended_reason }}</template>
        </div>
        <div v-else-if="summary?.low_balance" class="flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-300 light:text-amber-700">
          <AlertTriangle class="h-4 w-4 shrink-0" />{{ $t('wallet.lowBalanceBanner', { balance: money(walletStore.balance) }) }}
        </div>

        <div class="grid gap-4 lg:grid-cols-3">
          <Card class="lg:col-span-1">
            <CardHeader class="pb-2">
              <CardDescription>{{ $t('wallet.balance') }}</CardDescription>
              <CardTitle :class="walletStore.balance <= 0 ? 'text-4xl tabular-nums text-red-400' : 'text-4xl tabular-nums'">{{ money(walletStore.balance) }}</CardTitle>
            </CardHeader>
            <CardContent class="space-y-3 text-sm">
              <div class="flex justify-between"><span class="text-muted-foreground">{{ $t('wallet.spentThisMonth') }}</span><span class="tabular-nums">{{ money(summary?.spent_this_month || 0) }}</span></div>
              <div v-if="summary?.wallet.credit_limit" class="flex justify-between"><span class="text-muted-foreground">{{ $t('owner.creditLimit') }}</span><span class="tabular-nums">{{ money(summary.wallet.credit_limit) }}</span></div>
              <div class="flex justify-between"><span class="text-muted-foreground">{{ $t('owner.totalRecharged') }}</span><span class="tabular-nums">{{ money(summary?.wallet.total_credited || 0) }}</span></div>
              <div class="flex gap-2 rounded-md bg-white/[0.04] light:bg-gray-50 p-3 text-xs text-muted-foreground">
                <Info class="h-4 w-4 shrink-0" />{{ $t('wallet.rechargeHint') }}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader class="pb-2">
              <CardTitle class="text-base">{{ $t('owner.planUsage') }}</CardTitle>
              <CardDescription>
                {{ summary?.usage.plan?.name || $t('owner.noPlan') }}
                <template v-if="summary?.usage.plan_expires_at"> · {{ $t('owner.expiresOn', { d: formatDate(summary.usage.plan_expires_at) }) }}</template>
              </CardDescription>
            </CardHeader>
            <CardContent class="space-y-4">
              <div v-for="row in usageRows" :key="row.label" class="space-y-1">
                <div class="flex justify-between text-sm">
                  <span class="text-muted-foreground">{{ row.label }}</span>
                  <span class="tabular-nums">{{ row.used.toLocaleString() }} / {{ row.limit ? row.limit.toLocaleString() : '∞' }}</span>
                </div>
                <Progress v-if="row.limit" :model-value="Math.min(100, (row.used / row.limit) * 100)" class="h-1.5" />
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader class="pb-2">
              <CardTitle class="text-base">{{ $t('owner.spendByCategory') }}</CardTitle>
              <CardDescription>{{ $t('owner.thisMonth') }}</CardDescription>
            </CardHeader>
            <CardContent>
              <p v-if="!summary?.spend_by_category?.length" class="text-sm text-muted-foreground">{{ $t('owner.noSpendYet') }}</p>
              <ul v-else class="space-y-2">
                <li v-for="c in summary.spend_by_category" :key="c.category" class="flex justify-between text-sm">
                  <span>{{ c.category }} <span class="text-muted-foreground">× {{ c.count.toLocaleString() }}</span></span>
                  <span class="tabular-nums">{{ money(c.amount) }}</span>
                </li>
              </ul>
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader>
            <CardTitle class="text-base">{{ $t('wallet.yourRates') }}</CardTitle>
            <CardDescription>{{ $t('wallet.yourRatesDesc') }}</CardDescription>
          </CardHeader>
          <CardContent>
            <DataTable :items="rates" :columns="rateColumns" :empty-title="$t('wallet.noRates')">
              <template #cell-country="{ item }">
                <span class="font-mono text-xs mr-2">{{ item.country_code === '*' ? '*' : '+' + item.country_code }}</span>{{ item.country_name || '—' }}
              </template>
              <template #cell-category="{ item }"><Badge variant="secondary">{{ item.category }}</Badge></template>
              <template #cell-price="{ item }"><span class="tabular-nums">{{ money(item.price) }}</span></template>
            </DataTable>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle class="text-base">{{ $t('wallet.history') }}</CardTitle>
            <CardDescription>{{ $t('wallet.historyDesc') }}</CardDescription>
          </CardHeader>
          <CardContent>
            <TransactionsTable ref="txTable" scope="mine" :currency="walletStore.currency" />
          </CardContent>
        </Card>
      </div>
    </ScrollArea>
  </div>
</template>
