<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Skeleton } from '@/components/ui/skeleton'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { PageHeader, CrudFormDialog, DeleteConfirmDialog, IconButton, ErrorState } from '@/components/shared'
import { adminService, type Plan } from '@/services/api'
import { useCrudState } from '@/composables/useCrudState'
import { useWalletStore, formatMoney } from '@/stores/wallet'
import { getErrorMessage } from '@/lib/api-utils'
import { toast } from 'vue-sonner'
import { Package, Plus, Pencil, Trash2, Users, Phone, MessageSquare } from 'lucide-vue-next'

const { t } = useI18n()
const walletStore = useWalletStore()

type PlanForm = Omit<Plan, 'id' | 'clients_count' | 'created_at'>
const defaultForm: PlanForm = {
  name: '', description: '', price: 0, billing_cycle: 'monthly',
  max_users: 0, max_accounts: 0, max_monthly_messages: 0, is_default: false, is_active: true,
}

const plans = ref<Plan[]>([])
const isLoading = ref(true)
const error = ref(false)
const isDeleting = ref(false)
const {
  isSubmitting, isDialogOpen, editingItem, deleteDialogOpen, itemToDelete,
  formData, openCreateDialog, openEditDialog: baseOpenEdit, openDeleteDialog, closeDialog, closeDeleteDialog,
} = useCrudState<Plan, PlanForm>(defaultForm)

async function load() {
  isLoading.value = true
  error.value = false
  try {
    plans.value = (await adminService.listPlans()).data.data.plans || []
  } catch (e) {
    error.value = true
    toast.error(getErrorMessage(e, t('owner.loadFailed')))
  } finally {
    isLoading.value = false
  }
}
onMounted(load)

function openEdit(p: Plan) {
  baseOpenEdit(p, (x) => ({
    name: x.name, description: x.description, price: x.price, billing_cycle: x.billing_cycle,
    max_users: x.max_users, max_accounts: x.max_accounts, max_monthly_messages: x.max_monthly_messages,
    is_default: x.is_default, is_active: x.is_active,
  }))
}

async function save() {
  if (!formData.value.name.trim()) {
    toast.error(t('owner.planNameRequired'))
    return
  }
  const payload = {
    ...formData.value,
    price: Number(formData.value.price) || 0,
    max_users: Number(formData.value.max_users) || 0,
    max_accounts: Number(formData.value.max_accounts) || 0,
    max_monthly_messages: Number(formData.value.max_monthly_messages) || 0,
  }
  isSubmitting.value = true
  try {
    if (editingItem.value) {
      await adminService.updatePlan(editingItem.value.id, payload)
    } else {
      await adminService.createPlan(payload)
    }
    toast.success(t('owner.planSaved'))
    closeDialog()
    await load()
  } catch (e) {
    toast.error(getErrorMessage(e, t('owner.planSaveFailed')))
  } finally {
    isSubmitting.value = false
  }
}

async function confirmDelete() {
  if (!itemToDelete.value) return
  isDeleting.value = true
  try {
    await adminService.deletePlan(itemToDelete.value.id)
    toast.success(t('owner.planDeleted'))
    closeDeleteDialog()
    await load()
  } catch (e) {
    toast.error(getErrorMessage(e, t('owner.planDeleteFailed')))
  } finally {
    isDeleting.value = false
  }
}

const limit = (v: number) => (v > 0 ? v.toLocaleString() : t('owner.unlimited'))
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('owner.plansTitle')" :description="$t('owner.plansSubtitle')" :icon="Package">
      <template #actions>
        <Button size="sm" @click="openCreateDialog"><Plus class="h-4 w-4 mr-2" />{{ $t('owner.addPlan') }}</Button>
      </template>
    </PageHeader>

    <ErrorState v-if="error && !isLoading" :title="$t('common.loadErrorTitle')" :description="$t('common.loadErrorDescription')" :retry-label="$t('common.retryLoad')" class="flex-1" @retry="load" />

    <ScrollArea v-else class="flex-1">
      <div class="p-6">
        <div v-if="isLoading" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          <Skeleton v-for="i in 3" :key="i" class="h-64 rounded-xl" />
        </div>

        <Card v-else-if="!plans.length">
          <CardContent class="py-16 text-center space-y-3">
            <Package class="h-10 w-10 mx-auto text-muted-foreground" />
            <p class="font-medium">{{ $t('owner.noPlansYet') }}</p>
            <p class="text-sm text-muted-foreground">{{ $t('owner.noPlansYetDesc') }}</p>
            <Button variant="outline" size="sm" @click="openCreateDialog"><Plus class="h-4 w-4 mr-2" />{{ $t('owner.addPlan') }}</Button>
          </CardContent>
        </Card>

        <div v-else class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          <Card v-for="p in plans" :key="p.id" :class="p.is_active ? '' : 'opacity-60'">
            <CardHeader>
              <div class="flex items-start justify-between gap-2">
                <div>
                  <CardTitle class="text-lg">{{ p.name }}</CardTitle>
                  <CardDescription v-if="p.description" class="mt-1">{{ p.description }}</CardDescription>
                </div>
                <div class="flex gap-1">
                  <IconButton :icon="Pencil" :label="$t('common.edit')" class="h-8 w-8" @click="openEdit(p)" />
                  <IconButton :label="$t('common.delete')" class="h-8 w-8" @click="openDeleteDialog(p)"><Trash2 class="h-4 w-4 text-destructive" /></IconButton>
                </div>
              </div>
            </CardHeader>
            <CardContent class="space-y-4">
              <div>
                <span class="text-3xl font-semibold tabular-nums">{{ formatMoney(p.price, walletStore.currency) }}</span>
                <span class="text-sm text-muted-foreground"> / {{ p.billing_cycle === 'yearly' ? $t('owner.year') : $t('owner.month') }}</span>
              </div>
              <ul class="space-y-2 text-sm">
                <li class="flex items-center gap-2"><Users class="h-4 w-4 text-muted-foreground" />{{ $t('owner.usersLimit', { n: limit(p.max_users) }, p.max_users === 1 ? 1 : 2) }}</li>
                <li class="flex items-center gap-2"><Phone class="h-4 w-4 text-muted-foreground" />{{ $t('owner.numbersLimit', { n: limit(p.max_accounts) }, p.max_accounts === 1 ? 1 : 2) }}</li>
                <li class="flex items-center gap-2"><MessageSquare class="h-4 w-4 text-muted-foreground" />{{ $t('owner.messagesLimit', { n: limit(p.max_monthly_messages) }, p.max_monthly_messages === 1 ? 1 : 2) }}</li>
              </ul>
              <div class="flex items-center gap-2 pt-2 border-t border-white/[0.08] light:border-gray-200">
                <Badge variant="secondary">{{ $t('owner.nClients', { n: p.clients_count || 0 }, p.clients_count || 0) }}</Badge>
                <Badge v-if="p.is_default">{{ $t('owner.default') }}</Badge>
                <Badge v-if="!p.is_active" variant="secondary">{{ $t('common.inactive') }}</Badge>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>

    <CrudFormDialog
      v-model:open="isDialogOpen"
      :is-editing="!!editingItem"
      :is-submitting="isSubmitting"
      :edit-title="$t('owner.editPlan')"
      :create-title="$t('owner.addPlan')"
      :create-description="$t('owner.planFormDesc')"
      :edit-description="$t('owner.planFormDesc')"
      max-width="max-w-lg"
      @submit="save"
    >
      <div class="grid grid-cols-2 gap-3">
        <div class="space-y-2 col-span-2">
          <Label>{{ $t('common.name') }} <span class="text-destructive">*</span></Label>
          <Input v-model="formData.name" placeholder="Starter" />
        </div>
        <div class="space-y-2 col-span-2">
          <Label>{{ $t('common.description') }}</Label>
          <Textarea v-model="formData.description" :rows="2" />
        </div>
        <div class="space-y-2">
          <Label>{{ $t('owner.price') }} ({{ walletStore.currency }})</Label>
          <Input v-model.number="formData.price" type="number" min="0" step="0.01" />
        </div>
        <div class="space-y-2">
          <Label>{{ $t('owner.billingCycle') }}</Label>
          <Select v-model="formData.billing_cycle">
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="monthly">{{ $t('owner.monthly') }}</SelectItem>
              <SelectItem value="yearly">{{ $t('owner.yearly') }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="space-y-2">
          <Label>{{ $t('owner.maxUsers') }}</Label>
          <Input v-model.number="formData.max_users" type="number" min="0" />
        </div>
        <div class="space-y-2">
          <Label>{{ $t('owner.maxNumbers') }}</Label>
          <Input v-model.number="formData.max_accounts" type="number" min="0" />
        </div>
        <div class="space-y-2 col-span-2">
          <Label>{{ $t('owner.maxMonthlyMessages') }}</Label>
          <Input v-model.number="formData.max_monthly_messages" type="number" min="0" />
          <p class="text-xs text-muted-foreground">{{ $t('owner.zeroUnlimited') }}</p>
        </div>
        <div class="flex items-center justify-between col-span-2 rounded-lg border border-white/[0.08] light:border-gray-200 px-3 py-2">
          <div>
            <p class="text-sm font-medium">{{ $t('owner.defaultPlanToggle') }}</p>
            <p class="text-xs text-muted-foreground">{{ $t('owner.defaultPlanHint') }}</p>
          </div>
          <Switch v-model:checked="formData.is_default" />
        </div>
        <div class="flex items-center justify-between col-span-2 rounded-lg border border-white/[0.08] light:border-gray-200 px-3 py-2">
          <p class="text-sm font-medium">{{ $t('common.active') }}</p>
          <Switch v-model:checked="formData.is_active" />
        </div>
      </div>
    </CrudFormDialog>

    <DeleteConfirmDialog v-model:open="deleteDialogOpen" :title="$t('owner.deletePlan')" :item-name="itemToDelete?.name" :is-submitting="isDeleting" @confirm="confirmDelete">
      <p class="text-sm text-muted-foreground">{{ $t('owner.deletePlanWarning') }}</p>
    </DeleteConfirmDialog>
  </div>
</template>
