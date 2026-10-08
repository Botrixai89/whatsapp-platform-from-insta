<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { adminService } from '@/services/api'
import { formatMoney } from '@/stores/wallet'
import { getErrorMessage } from '@/lib/api-utils'
import { toast } from 'vue-sonner'
import { Loader2 } from 'lucide-vue-next'

/** Owner-only dialog to add funds to or deduct from a client's wallet. */
const props = withDefaults(defineProps<{
  orgId: string
  type: 'credit' | 'debit'
  clientName?: string
  balance?: number
  currency?: string
}>(), {
  balance: 0,
  currency: 'INR',
})

const open = defineModel<boolean>('open', { default: false })
const emit = defineEmits<{ done: [balance: number] }>()

const { t } = useI18n()
const amount = ref<string | number>('')
const source = ref('recharge')
const description = ref('')
const isSubmitting = ref(false)

watch(open, (isOpen) => {
  if (isOpen) {
    amount.value = ''
    source.value = props.type === 'credit' ? 'recharge' : 'manual'
    description.value = ''
  }
})

async function submit() {
  const value = Number(String(amount.value).replace(/,/g, ''))
  if (!value || value <= 0 || !Number.isFinite(value)) {
    toast.error(t('owner.amountRequired'))
    return
  }
  isSubmitting.value = true
  try {
    const res = await adminService.adjustWallet(props.orgId, {
      type: props.type,
      amount: value,
      source: props.type === 'credit' ? source.value : 'manual',
      description: description.value,
    })
    toast.success(props.type === 'credit' ? t('owner.walletCredited') : t('owner.walletDebited'))
    open.value = false
    emit('done', res.data.data.balance)
  } catch (e) {
    toast.error(getErrorMessage(e, t('owner.walletAdjustFailed')))
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="max-w-md">
      <DialogHeader>
        <DialogTitle>{{ type === 'credit' ? $t('owner.addFunds') : $t('owner.deduct') }}<template v-if="clientName"> — {{ clientName }}</template></DialogTitle>
        <DialogDescription>{{ $t('owner.currentBalance', { v: formatMoney(balance, currency) }) }}</DialogDescription>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="submit">
        <div class="space-y-2">
          <Label>{{ $t('owner.amount') }} ({{ currency }})</Label>
          <Input v-model="amount" type="number" inputmode="decimal" min="0" step="0.01" placeholder="0.00" autofocus />
        </div>
        <div v-if="type === 'credit'" class="space-y-2">
          <Label>{{ $t('owner.source') }}</Label>
          <Select v-model="source">
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="recharge">{{ $t('owner.sourceRecharge') }}</SelectItem>
              <SelectItem value="bonus">{{ $t('owner.sourceBonus') }}</SelectItem>
              <SelectItem value="manual">{{ $t('owner.sourceManual') }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="space-y-2">
          <Label>{{ $t('owner.noteOptional') }}</Label>
          <Input v-model="description" :placeholder="$t('owner.notePlaceholder')" />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="open = false">{{ $t('common.cancel') }}</Button>
          <Button type="submit" :variant="type === 'debit' ? 'destructive' : 'default'" :disabled="isSubmitting">
            <Loader2 v-if="isSubmitting" class="h-4 w-4 mr-2 animate-spin" />{{ type === 'credit' ? $t('owner.addFunds') : $t('owner.deduct') }}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>
