<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { DataTable, type Column } from '@/components/shared'
import { adminService, walletService, type WalletTransaction } from '@/services/api'
import { formatMoney } from '@/stores/wallet'
import { formatDateTime } from '@/lib/utils'
import { Receipt } from 'lucide-vue-next'

/**
 * Wallet ledger table.
 * - clientId set: one client's ledger (Owner Panel)
 * - scope="all": every client's ledger (Owner Panel)
 * - scope="mine": the current organization's own ledger (client wallet page)
 */
const props = withDefaults(defineProps<{
  clientId?: string
  scope?: 'client' | 'all' | 'mine'
  currency?: string
  organizationId?: string
}>(), {
  scope: 'client',
  currency: 'INR',
})

const { t } = useI18n()
const items = ref<WalletTransaction[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const isLoading = ref(false)
const typeFilter = ref('all')
const sourceFilter = ref('all')

const columns = computed<Column<WalletTransaction>[]>(() => {
  const cols: Column<WalletTransaction>[] = [{ key: 'created_at', label: t('common.date') }]
  if (props.scope === 'all') cols.push({ key: 'organization_name', label: t('owner.client') })
  cols.push(
    { key: 'description', label: t('common.description') },
    { key: 'source', label: t('owner.source') },
    { key: 'amount', label: t('owner.amount'), align: 'right' },
    { key: 'balance_after', label: t('owner.balanceAfter'), align: 'right' },
  )
  return cols
})

async function load() {
  isLoading.value = true
  try {
    const params = {
      page: page.value,
      limit: pageSize,
      type: typeFilter.value === 'all' ? undefined : typeFilter.value,
      source: sourceFilter.value === 'all' ? undefined : sourceFilter.value,
      organization_id: props.organizationId || undefined,
    }
    const res = props.scope === 'mine'
      ? await walletService.transactions(params)
      : props.scope === 'all'
        ? await adminService.transactions(params)
        : await adminService.clientTransactions(props.clientId!, params)
    items.value = res.data.data.transactions || []
    total.value = res.data.data.total
  } catch {
    items.value = []
  } finally {
    isLoading.value = false
  }
}

function reload() {
  page.value = 1
  load()
}

function onPage(p: number) {
  page.value = p
  load()
}

watch(() => [props.clientId, props.organizationId], reload)
onMounted(load)
defineExpose({ reload })

const sourceVariant = (s: string) => ({
  recharge: 'success', bonus: 'info', manual: 'secondary', message: 'secondary', refund: 'warning', adjustment: 'secondary',
} as Record<string, any>)[s] || 'secondary'
</script>

<template>
  <div class="space-y-3">
    <div class="flex flex-wrap gap-2 justify-end">
      <Select v-model="typeFilter" @update:model-value="reload">
        <SelectTrigger class="w-32"><SelectValue /></SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{{ $t('owner.allTypes') }}</SelectItem>
          <SelectItem value="credit">{{ $t('owner.credits') }}</SelectItem>
          <SelectItem value="debit">{{ $t('owner.debits') }}</SelectItem>
        </SelectContent>
      </Select>
      <Select v-model="sourceFilter" @update:model-value="reload">
        <SelectTrigger class="w-40"><SelectValue /></SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{{ $t('owner.allSources') }}</SelectItem>
          <SelectItem value="recharge">{{ $t('owner.sourceRecharge') }}</SelectItem>
          <SelectItem value="bonus">{{ $t('owner.sourceBonus') }}</SelectItem>
          <SelectItem value="manual">{{ $t('owner.sourceManual') }}</SelectItem>
          <SelectItem value="message">{{ $t('owner.sourceMessage') }}</SelectItem>
          <SelectItem value="refund">{{ $t('owner.sourceRefund') }}</SelectItem>
          <SelectItem value="adjustment">{{ $t('owner.sourceAdjustment') }}</SelectItem>
        </SelectContent>
      </Select>
    </div>
    <DataTable
      :items="items"
      :columns="columns"
      :is-loading="isLoading"
      :empty-icon="Receipt"
      :empty-title="$t('owner.noTransactions')"
      server-pagination
      :current-page="page"
      :total-items="total"
      :page-size="pageSize"
      item-name="transactions"
      @page-change="onPage"
    >
      <template #cell-created_at="{ item }"><span class="text-muted-foreground whitespace-nowrap">{{ formatDateTime(item.created_at) }}</span></template>
      <template #cell-organization_name="{ item }">
        <RouterLink :to="`/admin/clients/${item.organization_id}`" class="hover:underline">{{ item.organization_name }}</RouterLink>
      </template>
      <template #cell-description="{ item }">
        <div>{{ item.description }}</div>
        <div v-if="item.reference" class="text-xs text-muted-foreground font-mono truncate max-w-[260px]">{{ item.reference }}</div>
      </template>
      <template #cell-source="{ item }"><Badge :variant="sourceVariant(item.source)" class="capitalize">{{ item.source }}</Badge></template>
      <template #cell-amount="{ item }">
        <span :class="['tabular-nums font-medium', item.type === 'credit' ? 'text-emerald-400 light:text-emerald-600' : 'text-red-400 light:text-red-600']">
          {{ item.type === 'credit' ? '+' : '−' }}{{ formatMoney(item.amount, currency) }}
        </span>
      </template>
      <template #cell-balance_after="{ item }"><span class="tabular-nums">{{ formatMoney(item.balance_after, currency) }}</span></template>
    </DataTable>
  </div>
</template>
