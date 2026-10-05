<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { PageHeader, DataTable, CrudFormDialog, DeleteConfirmDialog, IconButton, ErrorState, type Column } from '@/components/shared'
import { adminService, type MessageRate, type AdminClient } from '@/services/api'
import { formatMoney } from '@/stores/wallet'
import { getErrorMessage } from '@/lib/api-utils'
import { toast } from 'vue-sonner'
import { IndianRupee, Plus, Pencil, Trash2, Info } from 'lucide-vue-next'

const { t } = useI18n()
const CATEGORIES = ['MARKETING', 'UTILITY', 'AUTHENTICATION', 'SERVICE'] as const

const rates = ref<MessageRate[]>([])
const currency = ref('INR')
const clients = ref<AdminClient[]>([])
const scope = ref('global')
const isLoading = ref(true)
const error = ref(false)

const columns = computed<Column<MessageRate>[]>(() => [
  { key: 'country', label: t('owner.country') },
  { key: 'category', label: t('owner.category') },
  { key: 'price', label: t('owner.pricePerMessage'), align: 'right' },
  { key: 'organization_name', label: t('owner.appliesTo') },
  { key: 'actions', label: t('common.actions'), align: 'right' },
])

async function load() {
  isLoading.value = true
  error.value = false
  try {
    const res = await adminService.listRates({ organization_id: scope.value === 'all' ? undefined : scope.value })
    rates.value = res.data.data.rates || []
    currency.value = res.data.data.currency || 'INR'
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
    clients.value = (await adminService.listClients({ limit: 100, sort: 'name', order: 'asc' })).data.data.clients || []
  } catch {
    clients.value = []
  }
})

const categoryVariant = (c: string) => ({ MARKETING: 'warning', UTILITY: 'info', AUTHENTICATION: 'success', SERVICE: 'secondary' } as Record<string, any>)[c] || 'secondary'

// Add country (all categories at once)
const addOpen = ref(false)
const isSubmitting = ref(false)
const addForm = ref({ organization_id: 'global', country_code: '', country_name: '', prices: { MARKETING: '', UTILITY: '', AUTHENTICATION: '', SERVICE: '' } as Record<string, string | number> })

function openAdd() {
  addForm.value = {
    organization_id: scope.value !== 'all' ? scope.value : 'global',
    country_code: '', country_name: '',
    prices: { MARKETING: '', UTILITY: '', AUTHENTICATION: '', SERVICE: '' },
  }
  addOpen.value = true
}

async function submitAdd() {
  const f = addForm.value
  if (!f.country_code.trim()) {
    toast.error(t('owner.countryCodeRequired'))
    return
  }
  const entries = CATEGORIES.filter(c => f.prices[c] !== '' && f.prices[c] !== null)
  if (!entries.length) {
    toast.error(t('owner.atLeastOnePrice'))
    return
  }
  isSubmitting.value = true
  let created = 0
  try {
    for (const c of entries) {
      await adminService.createRate({
        organization_id: f.organization_id === 'global' ? undefined : f.organization_id,
        country_code: f.country_code.trim(),
        country_name: f.country_name.trim(),
        category: c,
        price: Number(f.prices[c]),
      })
      created++
    }
    toast.success(t('owner.ratesAdded', { n: created }))
    addOpen.value = false
  } catch (e) {
    toast.error(getErrorMessage(e, t('owner.rateSaveFailed')))
  } finally {
    isSubmitting.value = false
    if (created) load()
  }
}

// Edit single rate
const editOpen = ref(false)
const editing = ref<MessageRate | null>(null)
const editForm = ref({ organization_id: 'global', country_code: '', country_name: '', category: 'MARKETING', price: 0 })

function openEdit(r: MessageRate) {
  editing.value = r
  editForm.value = {
    organization_id: r.organization_id || 'global',
    country_code: r.country_code,
    country_name: r.country_name,
    category: r.category,
    price: r.price,
  }
  editOpen.value = true
}

async function submitEdit() {
  if (!editing.value) return
  isSubmitting.value = true
  try {
    const f = editForm.value
    await adminService.updateRate(editing.value.id, {
      organization_id: f.organization_id === 'global' ? undefined : f.organization_id,
      country_code: f.country_code,
      country_name: f.country_name,
      category: f.category,
      price: Number(f.price),
    })
    toast.success(t('owner.rateSaved'))
    editOpen.value = false
    load()
  } catch (e) {
    toast.error(getErrorMessage(e, t('owner.rateSaveFailed')))
  } finally {
    isSubmitting.value = false
  }
}

// Delete
const deleteOpen = ref(false)
const toDelete = ref<MessageRate | null>(null)
const isDeleting = ref(false)
function openDelete(r: MessageRate) {
  toDelete.value = r
  deleteOpen.value = true
}
async function confirmDelete() {
  if (!toDelete.value) return
  isDeleting.value = true
  try {
    await adminService.deleteRate(toDelete.value.id)
    toast.success(t('owner.rateDeleted'))
    deleteOpen.value = false
    load()
  } catch (e) {
    toast.error(getErrorMessage(e, t('owner.rateDeleteFailed')))
  } finally {
    isDeleting.value = false
  }
}
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('owner.ratesTitle')" :description="$t('owner.ratesSubtitle')" :icon="IndianRupee" icon-gradient="bg-gradient-to-br from-amber-500 to-orange-600 shadow-amber-500/20">
      <template #actions>
        <Button size="sm" @click="openAdd"><Plus class="h-4 w-4 mr-2" />{{ $t('owner.addCountryRates') }}</Button>
      </template>
    </PageHeader>

    <ErrorState v-if="error && !isLoading" :title="$t('common.loadErrorTitle')" :description="$t('common.loadErrorDescription')" :retry-label="$t('common.retryLoad')" class="flex-1" @retry="load" />

    <ScrollArea v-else class="flex-1">
      <div class="p-6 max-w-6xl mx-auto space-y-4">
        <div class="flex gap-3 rounded-lg border border-sky-500/30 bg-sky-500/10 px-4 py-3 text-sm text-sky-200 light:text-sky-800">
          <Info class="h-4 w-4 shrink-0 mt-0.5" />
          <div class="space-y-1">
            <p>{{ $t('owner.ratesHelp1') }}</p>
            <p>{{ $t('owner.ratesHelp2') }}</p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <div class="flex items-center justify-between flex-wrap gap-4">
              <div>
                <CardTitle>{{ $t('owner.rateCard') }}</CardTitle>
                <CardDescription>{{ $t('owner.rateCardDesc', { c: currency }) }}</CardDescription>
              </div>
              <Select v-model="scope" @update:model-value="load">
                <SelectTrigger class="w-56"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="global">{{ $t('owner.globalRates') }}</SelectItem>
                  <SelectItem value="all">{{ $t('owner.allRates') }}</SelectItem>
                  <SelectItem v-for="c in clients" :key="c.id" :value="c.id">{{ $t('owner.customFor', { name: c.name }) }}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </CardHeader>
          <CardContent>
            <DataTable :items="rates" :columns="columns" :is-loading="isLoading" :empty-icon="IndianRupee" :empty-title="$t('owner.noRatesYet')" :empty-description="$t('owner.noRatesYetDesc')">
              <template #cell-country="{ item }">
                <span class="font-mono text-xs mr-2">{{ item.country_code === '*' ? '*' : '+' + item.country_code }}</span>{{ item.country_name || '—' }}
              </template>
              <template #cell-category="{ item }"><Badge :variant="categoryVariant(item.category)">{{ item.category }}</Badge></template>
              <template #cell-price="{ item }"><span class="tabular-nums font-medium">{{ formatMoney(item.price, currency) }}</span></template>
              <template #cell-organization_name="{ item }">
                <span v-if="item.organization_id">{{ item.organization_name }}</span>
                <span v-else class="text-muted-foreground">{{ $t('owner.allClientsLabel') }}</span>
              </template>
              <template #cell-actions="{ item }">
                <div class="flex justify-end gap-1">
                  <IconButton :icon="Pencil" :label="$t('common.edit')" class="h-8 w-8" @click="openEdit(item)" />
                  <IconButton :label="$t('common.delete')" class="h-8 w-8" @click="openDelete(item)"><Trash2 class="h-4 w-4 text-destructive" /></IconButton>
                </div>
              </template>
              <template #empty-action>
                <Button variant="outline" size="sm" @click="openAdd"><Plus class="h-4 w-4 mr-2" />{{ $t('owner.addCountryRates') }}</Button>
              </template>
            </DataTable>
          </CardContent>
        </Card>
      </div>
    </ScrollArea>

    <!-- Add country -->
    <CrudFormDialog v-model:open="addOpen" :is-submitting="isSubmitting" :create-title="$t('owner.addCountryRates')" :create-description="$t('owner.addCountryRatesDesc')" max-width="max-w-lg" @submit="submitAdd">
      <div class="grid grid-cols-2 gap-3">
        <div class="space-y-2 col-span-2">
          <Label>{{ $t('owner.appliesTo') }}</Label>
          <Select v-model="addForm.organization_id">
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="global">{{ $t('owner.allClientsLabel') }}</SelectItem>
              <SelectItem v-for="c in clients" :key="c.id" :value="c.id">{{ c.name }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="space-y-2">
          <Label>{{ $t('owner.countryCode') }} <span class="text-destructive">*</span></Label>
          <Input v-model="addForm.country_code" placeholder="91" />
          <p class="text-xs text-muted-foreground">{{ $t('owner.countryCodeHint') }}</p>
        </div>
        <div class="space-y-2">
          <Label>{{ $t('owner.countryName') }}</Label>
          <Input v-model="addForm.country_name" placeholder="India" />
        </div>
        <div v-for="c in CATEGORIES" :key="c" class="space-y-2">
          <Label>{{ c }} ({{ currency }})</Label>
          <Input v-model="addForm.prices[c]" type="number" min="0" step="0.0001" :placeholder="$t('owner.leaveEmptySkip')" />
        </div>
      </div>
    </CrudFormDialog>

    <!-- Edit single -->
    <CrudFormDialog v-model:open="editOpen" :is-editing="true" :is-submitting="isSubmitting" :edit-title="$t('owner.editRate')" max-width="max-w-md" @submit="submitEdit">
      <div class="grid grid-cols-2 gap-3">
        <div class="space-y-2 col-span-2">
          <Label>{{ $t('owner.appliesTo') }}</Label>
          <Select v-model="editForm.organization_id">
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="global">{{ $t('owner.allClientsLabel') }}</SelectItem>
              <SelectItem v-for="c in clients" :key="c.id" :value="c.id">{{ c.name }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="space-y-2">
          <Label>{{ $t('owner.countryCode') }}</Label>
          <Input v-model="editForm.country_code" />
        </div>
        <div class="space-y-2">
          <Label>{{ $t('owner.countryName') }}</Label>
          <Input v-model="editForm.country_name" />
        </div>
        <div class="space-y-2">
          <Label>{{ $t('owner.category') }}</Label>
          <Select v-model="editForm.category">
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem v-for="c in CATEGORIES" :key="c" :value="c">{{ c }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="space-y-2">
          <Label>{{ $t('owner.pricePerMessage') }}</Label>
          <Input v-model.number="editForm.price" type="number" min="0" step="0.0001" />
        </div>
      </div>
    </CrudFormDialog>

    <DeleteConfirmDialog v-model:open="deleteOpen" :title="$t('owner.deleteRate')" :item-name="toDelete ? `${toDelete.country_name || toDelete.country_code} · ${toDelete.category}` : ''" :is-submitting="isDeleting" @confirm="confirmDelete" />
  </div>
</template>
