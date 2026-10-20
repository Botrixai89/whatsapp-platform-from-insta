<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { paymentsService, type PaymentConfig } from '@/services/api'
import { formatMoney } from '@/stores/wallet'
import { getErrorMessage } from '@/lib/api-utils'
import { toast } from 'vue-sonner'
import { Loader2, ShieldCheck } from 'lucide-vue-next'

/** Self-serve wallet top-up through Razorpay Checkout (UPI, cards, net banking). */
const open = defineModel<boolean>('open', { required: true })
const props = defineProps<{ config: PaymentConfig }>()
const emit = defineEmits<{ paid: [] }>()
const { t } = useI18n()

const presets = [500, 1000, 2000, 5000]
const amount = ref<number>(1000)
const isPaying = ref(false)
watch(open, (o) => { if (o) amount.value = Math.max(1000, props.config.min_recharge) })

const valid = computed(() => amount.value >= props.config.min_recharge && amount.value <= props.config.max_recharge)

let checkoutScript: Promise<void> | null = null
function loadCheckout(): Promise<void> {
  if ((window as any).Razorpay) return Promise.resolve()
  checkoutScript ??= new Promise((resolve, reject) => {
    const s = document.createElement('script')
    s.src = 'https://checkout.razorpay.com/v1/checkout.js'
    s.onload = () => resolve()
    s.onerror = () => { checkoutScript = null; reject(new Error('checkout')) }
    document.head.appendChild(s)
  })
  return checkoutScript
}

async function pay() {
  if (!valid.value || isPaying.value) return
  isPaying.value = true
  try {
    const [order] = await Promise.all([paymentsService.createOrder(amount.value).then(r => r.data.data), loadCheckout()])
    const checkout = new (window as any).Razorpay({
      key: order.key_id,
      order_id: order.order_id,
      amount: order.amount,
      currency: order.currency,
      name: order.name,
      description: `${order.description} · ${order.business}`,
      prefill: order.prefill,
      theme: { color: '#10b981' },
      handler: async (resp: { razorpay_order_id: string; razorpay_payment_id: string; razorpay_signature: string }) => {
        try {
          await paymentsService.verify(resp)
          toast.success(t('wallet.rechargeSuccess', { amount: formatMoney(amount.value, order.currency) }))
          open.value = false
          emit('paid')
        } catch (e) {
          // The webhook will still credit it; tell the user what to expect
          toast.error(getErrorMessage(e, t('wallet.rechargeVerifyPending')))
        } finally {
          isPaying.value = false
        }
      },
      modal: { ondismiss: () => { isPaying.value = false } },
    })
    checkout.on('payment.failed', (r: any) => {
      toast.error(r?.error?.description || t('wallet.rechargeFailed'))
      isPaying.value = false
    })
    checkout.open()
  } catch (e) {
    toast.error(getErrorMessage(e, t('wallet.rechargeStartFailed')))
    isPaying.value = false
  }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="sm:max-w-sm">
      <DialogHeader>
        <DialogTitle>{{ $t('wallet.addMoney') }}</DialogTitle>
        <DialogDescription>{{ $t('wallet.addMoneyDesc') }}</DialogDescription>
      </DialogHeader>
      <div class="space-y-4">
        <div class="grid grid-cols-4 gap-2">
          <Button
            v-for="p in presets.filter(v => v >= config.min_recharge && v <= config.max_recharge)"
            :key="p"
            type="button"
            size="sm"
            :variant="amount === p ? 'default' : 'outline'"
            @click="amount = p"
          >{{ formatMoney(p, config.currency) }}</Button>
        </div>
        <div class="space-y-2">
          <Label for="recharge-amount">{{ $t('wallet.otherAmount') }}</Label>
          <Input id="recharge-amount" v-model.number="amount" type="number" :min="config.min_recharge" :max="config.max_recharge" step="1" />
          <p class="text-xs text-muted-foreground">
            {{ $t('wallet.rechargeLimits', { min: formatMoney(config.min_recharge, config.currency), max: formatMoney(config.max_recharge, config.currency) }) }}
          </p>
        </div>
        <p class="flex items-center gap-2 text-xs text-muted-foreground"><ShieldCheck class="h-4 w-4 shrink-0" />{{ $t('wallet.securePayment') }}</p>
      </div>
      <DialogFooter>
        <Button variant="outline" @click="open = false">{{ $t('common.cancel') }}</Button>
        <Button :disabled="!valid || isPaying" @click="pay">
          <Loader2 v-if="isPaying" class="h-4 w-4 animate-spin" />
          {{ $t('wallet.payAmount', { amount: formatMoney(amount || 0, config.currency) }) }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
