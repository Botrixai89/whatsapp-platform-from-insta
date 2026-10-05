import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { walletService, type WalletSummary } from '@/services/api'

/**
 * Current organization's wallet. Kept live by `wallet_update` WebSocket
 * events (see services/websocket.ts) with a periodic refresh as fallback,
 * since campaign workers debit without broadcasting.
 */
export const useWalletStore = defineStore('wallet', () => {
  const summary = ref<WalletSummary | null>(null)
  const isLoading = ref(false)
  let timer: ReturnType<typeof setInterval> | null = null

  const balance = computed(() => summary.value?.wallet.balance ?? 0)
  const currency = computed(() => summary.value?.wallet.currency ?? 'INR')
  const enabled = computed(() => summary.value?.enabled ?? false)
  const lowBalance = computed(() => summary.value?.low_balance ?? false)
  const suspended = computed(() => summary.value?.status === 'suspended')

  async function fetch() {
    isLoading.value = true
    try {
      const res = await walletService.get()
      summary.value = res.data.data
    } catch {
      // Wallet is optional UI; ignore failures
    } finally {
      isLoading.value = false
    }
  }

  function applyUpdate(payload: { balance: number; currency: string; low_balance: boolean }) {
    if (!summary.value) {
      fetch()
      return
    }
    summary.value.wallet.balance = payload.balance
    summary.value.wallet.currency = payload.currency
    summary.value.low_balance = payload.low_balance
  }

  function startPolling(intervalMs = 60000) {
    stopPolling()
    fetch()
    timer = setInterval(fetch, intervalMs)
  }

  function stopPolling() {
    if (timer) clearInterval(timer)
    timer = null
  }

  return { summary, isLoading, balance, currency, enabled, lowBalance, suspended, fetch, applyUpdate, startPolling, stopPolling }
})

export function formatMoney(amount: number, currency = 'INR'): string {
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency, minimumFractionDigits: 2, maximumFractionDigits: 4 }).format(amount)
  } catch {
    return `${currency} ${amount.toFixed(2)}`
  }
}
