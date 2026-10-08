<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Textarea } from '@/components/ui/textarea'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { PageHeader, ErrorState, DataTable, type Column } from '@/components/shared'
import TransactionsTable from './TransactionsTable.vue'
import WalletAdjustDialog from './WalletAdjustDialog.vue'
import { adminService, type AdminClientDetail, type Plan } from '@/services/api'
import { useOpenAsClient } from '@/composables/useOpenAsClient'
import { formatMoney } from '@/stores/wallet'
import { formatDate } from '@/lib/utils'
import { getErrorMessage } from '@/lib/api-utils'
import { Line } from '@/lib/charts'
import { toast } from 'vue-sonner'
import { Building2, Wallet, Plus, Minus, Ban, CheckCircle2, LogIn, Save, Users, Phone, Loader2 } from 'lucide-vue-next'

const { t } = useI18n()
const route = useRoute()
const { openAsClient } = useOpenAsClient()
const clientId = computed(() => route.params.id as string)

const detail = ref<AdminClientDetail | null>(null)
const plans = ref<Plan[]>([])
const isLoading = ref(true)
const error = ref(false)
const txTable = ref<InstanceType<typeof TransactionsTable> | null>(null)

const currency = computed(() => detail.value?.wallet?.currency || 'INR')
const money = (v: number) => formatMoney(v, currency.value)
const isSuspended = computed(() => detail.value?.client.status === 'suspended')

// Settings form
const settings = ref({
  name: '', contact_phone: '', notes: '', plan_id: 'none', plan_expires_at: '',
  credit_limit: 0, low_balance_threshold: 0,
})
const isSaving = ref(false)

function fillSettings(d: AdminClientDetail) {
  settings.value = {
    name: d.client.name,
    contact_phone: d.client.contact_phone || '',
    notes: d.organization.notes || '',
    plan_id: d.client.plan_id || 'none',
    plan_expires_at: d.client.plan_expires_at ? d.client.plan_expires_at.slice(0, 10) : '',
    credit_limit: d.wallet?.credit_limit || 0,
    low_balance_threshold: d.wallet?.low_balance_threshold || 0,
  }
}

async function load() {
  isLoading.value = true
  error.value = false
  try {
    const res = await adminService.getClient(clientId.value)
    detail.value = res.data.data
    fillSettings(res.data.data)
  } catch (e) {
    error.value = true
    toast.error(getErrorMessage(e, t('owner.loadFailed')))
  } finally {
    isLoading.value = false
  }
}

onMounted(async () => {
  load()
  try {
    plans.value = (await adminService.listPlans()).data.data.plans || []
  } catch {
    plans.value = []
  }
})

async function saveSettings() {
  isSaving.value = true
  try {
    const s = settings.value
    await adminService.updateClient(clientId.value, {
      name: s.name,
      contact_phone: s.contact_phone,
      notes: s.notes,
      plan_id: s.plan_id === 'none' ? '' : s.plan_id,
      plan_expires_at: s.plan_expires_at,
      credit_limit: Number(s.credit_limit) || 0,
      low_balance_threshold: Number(s.low_balance_threshold) || 0,
    })
    toast.success(t('owner.clientUpdated'))
    await load()
  } catch (e) {
    toast.error(getErrorMessage(e, t('owner.clientUpdateFailed')))
  } finally {
    isSaving.value = false
  }
}

// Suspend / activate
const statusDialogOpen = ref(false)
const suspendReason = ref('')
const isChangingStatus = ref(false)

async function changeStatus() {
  isChangingStatus.value = true
  try {
    const next = isSuspended.value ? 'active' : 'suspended'
    await adminService.updateClient(clientId.value, { status: next, suspended_reason: next === 'suspended' ? suspendReason.value : '' })
    toast.success(next === 'suspended' ? t('owner.clientSuspended') : t('owner.clientActivated'))
    statusDialogOpen.value = false
    suspendReason.value = ''
    await load()
  } catch (e) {
    toast.error(getErrorMessage(e, t('owner.clientUpdateFailed')))
  } finally {
    isChangingStatus.value = false
  }
}

// Wallet adjust
const walletDialogOpen = ref(false)
const walletType = ref<'credit' | 'debit'>('credit')

function openWallet(type: 'credit' | 'debit') {
  walletType.value = type
  walletDialogOpen.value = true
}

async function onWalletAdjusted() {
  await load()
  txTable.value?.reload()
}

// Usage
const usageRows = computed(() => {
  const u = detail.value?.usage
  if (!u) return []
  const plan = u.plan
  return [
    { label: t('owner.users'), used: u.users, limit: plan?.max_users || 0 },
    { label: t('owner.whatsappNumbers'), used: u.accounts, limit: plan?.max_accounts || 0 },
    { label: t('owner.messagesMonth'), used: u.messages_this_month, limit: plan?.max_monthly_messages || 0 },
  ]
})

const usageChart = computed(() => {
  const daily = detail.value?.daily || []
  return {
    labels: daily.map(d => d.date.slice(5)),
    datasets: [
      { label: t('owner.sent'), data: daily.map(d => d.outgoing), borderColor: '#10b981', backgroundColor: 'rgba(16,185,129,0.15)', fill: true, tension: 0.3, yAxisID: 'y' },
      { label: t('owner.received'), data: daily.map(d => d.incoming), borderColor: '#6366f1', tension: 0.3, yAxisID: 'y' },
      { label: t('owner.spend'), data: daily.map(d => d.spend), borderColor: '#f59e0b', borderDash: [4, 4], tension: 0.3, yAxisID: 'y1' },
    ]
  }
})
const usageChartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: { legend: { display: true, position: 'top' as const } },
  scales: {
    y: { beginAtZero: true, position: 'left' as const },
    y1: { beginAtZero: true, position: 'right' as const, grid: { drawOnChartArea: false } },
  }
}

const memberColumns = computed<Column<any>[]>(() => [
  { key: 'full_name', label: t('common.name') },
  { key: 'role_name', label: t('owner.role') },
  { key: 'is_active', label: t('common.status') },
  { key: 'created_at', label: t('owner.joined') },
])
const numberColumns = computed<Column<any>[]>(() => [
  { key: 'name', label: t('common.name') },
  { key: 'phone_id', label: t('owner.phoneId') },
  { key: 'status', label: t('common.status') },
  { key: 'created_at', label: t('owner.connected') },
])
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader
      :title="detail?.client.name || $t('owner.client')"
      :description="detail?.client.owner_email"
      :icon="Building2"
      icon-gradient="bg-gradient-to-br from-violet-500 to-purple-600 shadow-violet-500/20"
      back-link="/admin/clients"
    >
      <template #actions>
        <div v-if="detail" class="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" @click="openAsClient(clientId)"><LogIn class="h-4 w-4 mr-2" />{{ $t('owner.openAsClient') }}</Button>
          <Button :variant="isSuspended ? 'default' : 'destructive'" size="sm" @click="statusDialogOpen = true">
            <component :is="isSuspended ? CheckCircle2 : Ban" class="h-4 w-4 mr-2" />
            {{ isSuspended ? $t('owner.activate') : $t('owner.suspend') }}
          </Button>
        </div>
      </template>
    </PageHeader>

    <ErrorState v-if="error && !isLoading" :title="$t('common.loadErrorTitle')" :description="$t('common.loadErrorDescription')" :retry-label="$t('common.retryLoad')" class="flex-1" @retry="load" />

    <ScrollArea v-else class="flex-1">
      <div class="p-6 max-w-7xl mx-auto space-y-6">
        <template v-if="isLoading && !detail">
          <div class="grid gap-4 lg:grid-cols-3"><Skeleton v-for="i in 3" :key="i" class="h-48 rounded-xl" /></div>
          <Skeleton class="h-72 rounded-xl" />
        </template>

        <template v-if="detail">
          <div v-if="isSuspended" class="flex items-center gap-2 rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300 light:text-red-700">
            <Ban class="h-4 w-4 shrink-0" />
            {{ $t('owner.suspendedNotice') }}<template v-if="detail.organization.suspended_reason"> — {{ detail.organization.suspended_reason }}</template>
          </div>

          <div class="grid gap-4 lg:grid-cols-3">
            <!-- Wallet -->
            <Card>
              <CardHeader class="pb-2">
                <CardTitle class="text-base flex items-center gap-2"><Wallet class="h-4 w-4" />{{ $t('owner.wallet') }}</CardTitle>
              </CardHeader>
              <CardContent class="space-y-4">
                <div>
                  <p :class="['text-3xl font-semibold tabular-nums', detail.wallet.balance <= 0 ? 'text-red-400' : 'text-white light:text-gray-900']">{{ money(detail.wallet.balance) }}</p>
                  <p class="text-xs text-muted-foreground mt-1">{{ $t('owner.creditLimitValue', { v: money(detail.wallet.credit_limit) }) }}</p>
                </div>
                <div class="grid grid-cols-2 gap-3 text-sm">
                  <div>
                    <p class="text-xs text-muted-foreground">{{ $t('owner.totalRecharged') }}</p>
                    <p class="tabular-nums">{{ money(detail.wallet.total_credited) }}</p>
                  </div>
                  <div>
                    <p class="text-xs text-muted-foreground">{{ $t('owner.totalSpent') }}</p>
                    <p class="tabular-nums">{{ money(detail.wallet.total_spent) }}</p>
                  </div>
                  <div>
                    <p class="text-xs text-muted-foreground">{{ $t('owner.spendMonth') }}</p>
                    <p class="tabular-nums">{{ money(detail.client.spend_month) }}</p>
                  </div>
                  <div>
                    <p class="text-xs text-muted-foreground">{{ $t('owner.lowBalanceAt') }}</p>
                    <p class="tabular-nums">{{ money(detail.wallet.low_balance_threshold) }}</p>
                  </div>
                </div>
                <div class="flex gap-2">
                  <Button size="sm" class="flex-1" @click="openWallet('credit')"><Plus class="h-4 w-4 mr-1" />{{ $t('owner.addFunds') }}</Button>
                  <Button size="sm" variant="outline" class="flex-1" @click="openWallet('debit')"><Minus class="h-4 w-4 mr-1" />{{ $t('owner.deduct') }}</Button>
                </div>
              </CardContent>
            </Card>

            <!-- Plan & usage -->
            <Card>
              <CardHeader class="pb-2">
                <CardTitle class="text-base">{{ $t('owner.planUsage') }}</CardTitle>
                <CardDescription>
                  {{ detail.usage.plan?.name || $t('owner.noPlan') }}
                  <template v-if="detail.usage.plan_expires_at"> · {{ $t('owner.expiresOn', { d: formatDate(detail.usage.plan_expires_at) }) }}</template>
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

            <!-- Spend by category -->
            <Card>
              <CardHeader class="pb-2">
                <CardTitle class="text-base">{{ $t('owner.spendByCategory') }}</CardTitle>
                <CardDescription>{{ $t('owner.thisMonth') }}</CardDescription>
              </CardHeader>
              <CardContent>
                <p v-if="!detail.spend_by_category?.length" class="text-sm text-muted-foreground">{{ $t('owner.noSpendYet') }}</p>
                <ul v-else class="space-y-2">
                  <li v-for="c in detail.spend_by_category" :key="c.category" class="flex justify-between text-sm">
                    <span>{{ c.category }} <span class="text-muted-foreground">× {{ c.count.toLocaleString() }}</span></span>
                    <span class="tabular-nums">{{ money(c.amount) }}</span>
                  </li>
                </ul>
                <div class="grid grid-cols-3 gap-2 mt-4 pt-4 border-t border-white/[0.08] light:border-gray-200 text-center">
                  <div><p class="text-lg font-semibold">{{ detail.client.numbers_count }}</p><p class="text-xs text-muted-foreground">{{ $t('owner.numbers') }}</p></div>
                  <div><p class="text-lg font-semibold">{{ detail.client.users_count }}</p><p class="text-xs text-muted-foreground">{{ $t('owner.users') }}</p></div>
                  <div><p class="text-lg font-semibold">{{ detail.client.contacts_count.toLocaleString() }}</p><p class="text-xs text-muted-foreground">{{ $t('owner.contacts') }}</p></div>
                </div>
              </CardContent>
            </Card>
          </div>

          <!-- Usage chart -->
          <Card>
            <CardHeader><CardTitle class="text-base">{{ $t('owner.activityLast30') }}</CardTitle></CardHeader>
            <CardContent><div class="h-64"><Line :data="usageChart" :options="usageChartOptions" /></div></CardContent>
          </Card>

          <!-- Settings -->
          <Card>
            <CardHeader>
              <CardTitle class="text-base">{{ $t('owner.clientSettings') }}</CardTitle>
              <CardDescription>{{ $t('owner.clientSettingsDesc') }}</CardDescription>
            </CardHeader>
            <CardContent>
              <div class="grid gap-4 md:grid-cols-3">
                <div class="space-y-2">
                  <Label>{{ $t('owner.businessName') }}</Label>
                  <Input v-model="settings.name" />
                </div>
                <div class="space-y-2">
                  <Label>{{ $t('common.phone') }}</Label>
                  <Input v-model="settings.contact_phone" />
                </div>
                <div class="space-y-2">
                  <Label>{{ $t('owner.plan') }}</Label>
                  <Select v-model="settings.plan_id">
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">{{ $t('owner.noPlan') }}</SelectItem>
                      <SelectItem v-for="p in plans" :key="p.id" :value="p.id">{{ p.name }}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div class="space-y-2">
                  <Label>{{ $t('owner.planExpires') }}</Label>
                  <Input v-model="settings.plan_expires_at" type="date" />
                </div>
                <div class="space-y-2">
                  <Label>{{ $t('owner.creditLimit') }}</Label>
                  <Input v-model.number="settings.credit_limit" type="number" min="0" step="0.01" />
                  <p class="text-xs text-muted-foreground">{{ $t('owner.creditLimitHint') }}</p>
                </div>
                <div class="space-y-2">
                  <Label>{{ $t('owner.lowBalanceAt') }}</Label>
                  <Input v-model.number="settings.low_balance_threshold" type="number" min="0" step="0.01" />
                </div>
                <div class="space-y-2 md:col-span-3">
                  <Label>{{ $t('owner.internalNotes') }}</Label>
                  <Textarea v-model="settings.notes" :rows="3" :placeholder="$t('owner.internalNotesPlaceholder')" />
                </div>
              </div>
              <div class="flex justify-end mt-4">
                <Button size="sm" :disabled="isSaving" @click="saveSettings">
                  <Loader2 v-if="isSaving" class="h-4 w-4 mr-2 animate-spin" /><Save v-else class="h-4 w-4 mr-2" />{{ $t('common.save') }}
                </Button>
              </div>
            </CardContent>
          </Card>

          <div class="grid gap-4 lg:grid-cols-2">
            <Card>
              <CardHeader><CardTitle class="text-base flex items-center gap-2"><Users class="h-4 w-4" />{{ $t('owner.teamMembers') }}</CardTitle></CardHeader>
              <CardContent>
                <DataTable :items="detail.members || []" :columns="memberColumns" :empty-title="$t('owner.noMembers')">
                  <template #cell-full_name="{ item }">
                    <div class="font-medium">{{ item.full_name }}</div>
                    <div class="text-xs text-muted-foreground">{{ item.email }}</div>
                  </template>
                  <template #cell-role_name="{ item }"><span class="capitalize">{{ item.role_name || '—' }}</span></template>
                  <template #cell-is_active="{ item }">
                    <Badge :variant="item.is_active ? 'success' : 'secondary'">{{ item.is_active ? $t('common.active') : $t('common.inactive') }}</Badge>
                  </template>
                  <template #cell-created_at="{ item }"><span class="text-muted-foreground">{{ formatDate(item.created_at) }}</span></template>
                </DataTable>
              </CardContent>
            </Card>
            <Card>
              <CardHeader><CardTitle class="text-base flex items-center gap-2"><Phone class="h-4 w-4" />{{ $t('owner.whatsappNumbers') }}</CardTitle></CardHeader>
              <CardContent>
                <DataTable :items="detail.numbers || []" :columns="numberColumns" :empty-title="$t('owner.noNumbers')">
                  <template #cell-status="{ item }"><Badge :variant="item.status === 'active' ? 'success' : 'warning'" class="capitalize">{{ item.status }}</Badge></template>
                  <template #cell-phone_id="{ item }"><span class="font-mono text-xs">{{ item.phone_id }}</span></template>
                  <template #cell-created_at="{ item }"><span class="text-muted-foreground">{{ formatDate(item.created_at) }}</span></template>
                </DataTable>
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle class="text-base">{{ $t('owner.walletTransactions') }}</CardTitle>
              <CardDescription>{{ $t('owner.walletTransactionsDesc') }}</CardDescription>
            </CardHeader>
            <CardContent>
              <TransactionsTable ref="txTable" :client-id="clientId" :currency="currency" />
            </CardContent>
          </Card>
        </template>
      </div>
    </ScrollArea>

    <WalletAdjustDialog
      v-model:open="walletDialogOpen"
      :org-id="clientId"
      :type="walletType"
      :balance="detail?.wallet.balance || 0"
      :currency="currency"
      @done="onWalletAdjusted"
    />

    <!-- Suspend / activate dialog -->
    <Dialog v-model:open="statusDialogOpen">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>{{ isSuspended ? $t('owner.activateTitle') : $t('owner.suspendTitle') }}</DialogTitle>
          <DialogDescription>{{ isSuspended ? $t('owner.activateDesc') : $t('owner.suspendDesc') }}</DialogDescription>
        </DialogHeader>
        <div v-if="!isSuspended" class="space-y-2">
          <Label>{{ $t('owner.reasonShownToClient') }}</Label>
          <Input v-model="suspendReason" :placeholder="$t('owner.reasonPlaceholder')" />
        </div>
        <DialogFooter>
          <Button variant="outline" @click="statusDialogOpen = false">{{ $t('common.cancel') }}</Button>
          <Button :variant="isSuspended ? 'default' : 'destructive'" :disabled="isChangingStatus" @click="changeStatus">
            <Loader2 v-if="isChangingStatus" class="h-4 w-4 mr-2 animate-spin" />
            {{ isSuspended ? $t('owner.activate') : $t('owner.suspend') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
