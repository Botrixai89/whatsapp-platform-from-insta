<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { PageHeader, SearchInput, DataTable, CrudFormDialog, IconButton, ErrorState, type Column } from '@/components/shared'
import { adminService, type AdminClient, type Plan } from '@/services/api'
import { useSearchPagination } from '@/composables/useSearchPagination'
import { useOpenAsClient } from '@/composables/useOpenAsClient'
import { formatMoney } from '@/stores/wallet'
import { formatDate } from '@/lib/utils'
import { getErrorMessage } from '@/lib/api-utils'
import { toast } from 'vue-sonner'
import { Building2, Plus, Eye, LogIn } from 'lucide-vue-next'

const { t } = useI18n()
const router = useRouter()
const { openAsClient } = useOpenAsClient()

const clients = ref<AdminClient[]>([])
const plans = ref<Plan[]>([])
const isLoading = ref(false)
const error = ref(false)
const statusFilter = ref('all')
const planFilter = ref('all')
const sortKey = ref('created_at')
const sortDirection = ref<'asc' | 'desc'>('desc')

const { searchQuery, currentPage, totalItems, pageSize, handlePageChange, resetAndFetch } = useSearchPagination({
  fetchFn: () => fetchClients(),
})

const columns = computed<Column<AdminClient>[]>(() => [
  { key: 'name', label: t('owner.business'), sortable: true },
  { key: 'plan_name', label: t('owner.plan') },
  { key: 'status', label: t('common.status') },
  { key: 'balance', label: t('owner.balance'), sortable: true, align: 'right' },
  { key: 'numbers_count', label: t('owner.numbers'), align: 'center' },
  { key: 'users_count', label: t('owner.users'), align: 'center' },
  { key: 'messages_month', label: t('owner.messagesMonth'), sortable: true, align: 'right' },
  { key: 'spend_month', label: t('owner.spendMonth'), sortable: true, align: 'right' },
  { key: 'last_activity_at', label: t('owner.lastActive'), sortable: true },
  { key: 'actions', label: t('common.actions'), align: 'right' },
])

async function fetchClients() {
  isLoading.value = true
  error.value = false
  try {
    const res = await adminService.listClients({
      search: searchQuery.value || undefined,
      status: statusFilter.value === 'all' ? undefined : statusFilter.value,
      plan_id: planFilter.value === 'all' ? undefined : planFilter.value,
      sort: sortKey.value,
      order: sortDirection.value,
      page: currentPage.value,
      limit: pageSize,
    })
    clients.value = res.data.data.clients || []
    totalItems.value = res.data.data.total
  } catch (e) {
    error.value = true
    toast.error(getErrorMessage(e, t('owner.loadFailed')))
  } finally {
    isLoading.value = false
  }
}

async function fetchPlans() {
  try {
    const res = await adminService.listPlans()
    plans.value = res.data.data.plans || []
  } catch {
    plans.value = []
  }
}

function onSort() {
  resetAndFetch()
}

onMounted(() => {
  fetchClients()
  fetchPlans()
})

// Create client
const isDialogOpen = ref(false)
const isSubmitting = ref(false)
const emptyForm = () => ({
  name: '', contact_phone: '', owner_name: '', owner_email: '', owner_password: '',
  plan_id: 'default', plan_expires_at: '', initial_balance: 0,
})
const form = ref(emptyForm())

function openCreate() {
  form.value = emptyForm()
  isDialogOpen.value = true
}

async function createClient() {
  const f = form.value
  if (!f.name.trim() || !f.owner_name.trim() || !f.owner_email.trim()) {
    toast.error(t('owner.requiredFields'))
    return
  }
  if (f.owner_password.length < 8) {
    toast.error(t('owner.passwordTooShort'))
    return
  }
  isSubmitting.value = true
  try {
    const res = await adminService.createClient({
      ...f,
      plan_id: f.plan_id === 'default' ? undefined : f.plan_id,
      plan_expires_at: f.plan_expires_at || undefined,
      initial_balance: Number(f.initial_balance) || 0,
    })
    toast.success(t('owner.clientCreated'))
    isDialogOpen.value = false
    router.push(`/admin/clients/${res.data.data.id}`)
  } catch (e) {
    toast.error(getErrorMessage(e, t('owner.clientCreateFailed')))
  } finally {
    isSubmitting.value = false
  }
}

function relative(date: string | null) {
  if (!date) return t('owner.never')
  return formatDate(date)
}
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('owner.clientsTitle')" :description="$t('owner.clientsSubtitle')" :icon="Building2" icon-gradient="bg-gradient-to-br from-violet-500 to-purple-600 shadow-violet-500/20">
      <template #actions>
        <Button size="sm" @click="openCreate"><Plus class="h-4 w-4 mr-2" />{{ $t('owner.addClient') }}</Button>
      </template>
    </PageHeader>

    <ErrorState v-if="error && !isLoading" :title="$t('common.loadErrorTitle')" :description="$t('common.loadErrorDescription')" :retry-label="$t('common.retryLoad')" class="flex-1" @retry="fetchClients" />

    <ScrollArea v-else class="flex-1">
      <div class="p-6">
        <div class="max-w-7xl mx-auto">
          <Card>
            <CardHeader>
              <div class="flex items-center justify-between flex-wrap gap-4">
                <div>
                  <CardTitle>{{ $t('owner.allClients') }}</CardTitle>
                  <CardDescription>{{ $t('owner.allClientsDesc', { n: totalItems }) }}</CardDescription>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <Select v-model="statusFilter" @update:model-value="resetAndFetch">
                    <SelectTrigger class="w-36"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">{{ $t('owner.allStatuses') }}</SelectItem>
                      <SelectItem value="active">{{ $t('owner.statusActive') }}</SelectItem>
                      <SelectItem value="suspended">{{ $t('owner.statusSuspended') }}</SelectItem>
                    </SelectContent>
                  </Select>
                  <Select v-model="planFilter" @update:model-value="resetAndFetch">
                    <SelectTrigger class="w-40"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">{{ $t('owner.allPlans') }}</SelectItem>
                      <SelectItem value="none">{{ $t('owner.noPlan') }}</SelectItem>
                      <SelectItem v-for="p in plans" :key="p.id" :value="p.id">{{ p.name }}</SelectItem>
                    </SelectContent>
                  </Select>
                  <SearchInput v-model="searchQuery" :placeholder="$t('owner.searchClients')" class="w-64" />
                </div>
              </div>
            </CardHeader>
            <CardContent>
              <DataTable
                :items="clients"
                :columns="columns"
                :is-loading="isLoading"
                :empty-icon="Building2"
                :empty-title="searchQuery ? $t('owner.noMatchingClients') : $t('owner.noClientsYet')"
                :empty-description="$t('owner.noClientsYetDesc')"
                v-model:sort-key="sortKey"
                v-model:sort-direction="sortDirection"
                server-pagination
                :current-page="currentPage"
                :total-items="totalItems"
                :page-size="pageSize"
                item-name="clients"
                @sort="onSort"
                @page-change="handlePageChange"
              >
                <template #cell-name="{ item }">
                  <RouterLink :to="`/admin/clients/${item.id}`" class="block min-w-0 hover:underline">
                    <div class="font-medium text-white light:text-gray-900 truncate">{{ item.name }}</div>
                    <div class="text-xs text-muted-foreground truncate">{{ item.owner_email || item.contact_phone || '—' }}</div>
                  </RouterLink>
                </template>
                <template #cell-plan_name="{ item }">
                  <span v-if="item.plan_name">{{ item.plan_name }}</span>
                  <span v-else class="text-muted-foreground">—</span>
                </template>
                <template #cell-status="{ item }">
                  <Badge :variant="item.status === 'suspended' ? 'destructive' : 'success'">
                    {{ item.status === 'suspended' ? $t('owner.statusSuspended') : $t('owner.statusActive') }}
                  </Badge>
                </template>
                <template #cell-balance="{ item }">
                  <span :class="['tabular-nums', item.balance <= 0 ? 'text-red-400' : '']">{{ formatMoney(item.balance, item.currency) }}</span>
                </template>
                <template #cell-messages_month="{ item }">
                  <span class="tabular-nums">{{ item.messages_month.toLocaleString() }}</span>
                </template>
                <template #cell-spend_month="{ item }">
                  <span class="tabular-nums">{{ formatMoney(item.spend_month, item.currency) }}</span>
                </template>
                <template #cell-last_activity_at="{ item }">
                  <span class="text-muted-foreground">{{ relative(item.last_activity_at) }}</span>
                </template>
                <template #cell-actions="{ item }">
                  <div class="flex items-center justify-end gap-1">
                    <IconButton :icon="Eye" :label="$t('owner.viewDetails')" class="h-8 w-8" @click="router.push(`/admin/clients/${item.id}`)" />
                    <IconButton :icon="LogIn" :label="$t('owner.openAsClient')" class="h-8 w-8" @click="openAsClient(item.id)" />
                  </div>
                </template>
                <template #empty-action>
                  <Button variant="outline" size="sm" @click="openCreate"><Plus class="h-4 w-4 mr-2" />{{ $t('owner.addClient') }}</Button>
                </template>
              </DataTable>
            </CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>

    <CrudFormDialog
      v-model:open="isDialogOpen"
      :is-submitting="isSubmitting"
      :create-title="$t('owner.addClient')"
      :create-description="$t('owner.addClientDesc')"
      max-width="max-w-lg"
      @submit="createClient"
    >
      <div class="space-y-4">
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-2 col-span-2">
            <Label>{{ $t('owner.businessName') }} <span class="text-destructive">*</span></Label>
            <Input v-model="form.name" :placeholder="$t('owner.businessNamePlaceholder')" />
          </div>
          <div class="space-y-2">
            <Label>{{ $t('owner.ownerName') }} <span class="text-destructive">*</span></Label>
            <Input v-model="form.owner_name" />
          </div>
          <div class="space-y-2">
            <Label>{{ $t('common.phone') }}</Label>
            <Input v-model="form.contact_phone" placeholder="+91 98765 43210" />
          </div>
          <div class="space-y-2">
            <Label>{{ $t('owner.ownerEmail') }} <span class="text-destructive">*</span></Label>
            <Input v-model="form.owner_email" type="email" />
          </div>
          <div class="space-y-2">
            <Label>{{ $t('owner.ownerPassword') }} <span class="text-destructive">*</span></Label>
            <Input v-model="form.owner_password" type="password" autocomplete="new-password" />
          </div>
          <div class="space-y-2">
            <Label>{{ $t('owner.plan') }}</Label>
            <Select v-model="form.plan_id">
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="default">{{ $t('owner.defaultPlan') }}</SelectItem>
                <SelectItem v-for="p in plans" :key="p.id" :value="p.id">{{ p.name }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div class="space-y-2">
            <Label>{{ $t('owner.planExpires') }}</Label>
            <Input v-model="form.plan_expires_at" type="date" />
          </div>
          <div class="space-y-2 col-span-2">
            <Label>{{ $t('owner.openingBalance') }}</Label>
            <Input v-model.number="form.initial_balance" type="number" min="0" step="0.01" />
            <p class="text-xs text-muted-foreground">{{ $t('owner.openingBalanceHint') }}</p>
          </div>
        </div>
      </div>
    </CrudFormDialog>
  </div>
</template>
