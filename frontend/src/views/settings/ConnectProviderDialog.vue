<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { accountsService, type ConnectProviderResult } from '@/services/api'
import { getErrorMessage } from '@/lib/api-utils'
import { toast } from 'vue-sonner'
import { CheckCircle2, AlertTriangle, Copy, Loader2, Eye, EyeOff } from 'lucide-vue-next'

/** Connects a WhatsApp number that is managed through the BotrixAI API (Heltar white-label). */
const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ connected: [] }>()
const { t } = useI18n()

const DEFAULT_API_URL = 'https://newapi.botrixai.com'
const form = ref({ name: '', api_url: DEFAULT_API_URL, api_key: '' })
const showKey = ref(false)
const isSaving = ref(false)
const result = ref<ConnectProviderResult | null>(null)

watch(open, (isOpen) => {
  if (isOpen) {
    form.value = { name: '', api_url: DEFAULT_API_URL, api_key: '' }
    result.value = null
    showKey.value = false
  }
})

async function connect() {
  if (!form.value.name.trim() || !form.value.api_key.trim()) {
    toast.error(t('accounts.providerRequired'))
    return
  }
  isSaving.value = true
  try {
    const res = await accountsService.connectProvider({
      name: form.value.name.trim(),
      api_url: form.value.api_url.trim() || DEFAULT_API_URL,
      api_key: form.value.api_key.trim(),
    })
    result.value = res.data.data
    emit('connected')
    toast.success(t('accounts.providerConnected'))
  } catch (e) {
    toast.error(getErrorMessage(e, t('accounts.providerConnectFailed')))
  } finally {
    isSaving.value = false
  }
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(t('common.copied'))
  } catch {
    toast.error(t('common.clipboardFailed'))
  }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>{{ $t('accounts.providerTitle') }}</DialogTitle>
        <DialogDescription>{{ $t('accounts.providerDesc') }}</DialogDescription>
      </DialogHeader>

      <!-- Step 1: credentials -->
      <form v-if="!result" class="space-y-4" @submit.prevent="connect">
        <div class="space-y-2">
          <Label for="prov-name">{{ $t('accounts.accountName') }}</Label>
          <Input id="prov-name" v-model="form.name" :placeholder="$t('accounts.providerNamePlaceholder')" autocomplete="off" />
        </div>
        <div class="space-y-2">
          <Label for="prov-key">{{ $t('accounts.providerApiKey') }}</Label>
          <div class="relative">
            <Input
              id="prov-key"
              v-model="form.api_key"
              :type="showKey ? 'text' : 'password'"
              placeholder="hk_live_…"
              autocomplete="off"
              class="pr-10 font-mono"
            />
            <button
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-muted-foreground hover:text-foreground"
              :aria-label="showKey ? $t('accounts.hideKey') : $t('accounts.showKey')"
              @click="showKey = !showKey"
            >
              <component :is="showKey ? EyeOff : Eye" class="h-4 w-4" />
            </button>
          </div>
          <p class="text-xs text-muted-foreground">{{ $t('accounts.providerApiKeyHint') }}</p>
        </div>
        <div class="space-y-2">
          <Label for="prov-url">{{ $t('accounts.providerApiUrl') }}</Label>
          <Input id="prov-url" v-model="form.api_url" class="font-mono" />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="open = false">{{ $t('common.cancel') }}</Button>
          <Button type="submit" :disabled="isSaving">
            <Loader2 v-if="isSaving" class="h-4 w-4 animate-spin" />
            {{ isSaving ? $t('accounts.providerConnecting') : $t('accounts.providerConnect') }}
          </Button>
        </DialogFooter>
      </form>

      <!-- Step 2: result and webhook status -->
      <div v-else class="space-y-4">
        <div class="flex items-start gap-3 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3">
          <CheckCircle2 class="h-5 w-5 shrink-0 text-emerald-500" />
          <div class="text-sm">
            <p class="font-medium">{{ $t('accounts.providerConnectedTitle', { name: result.account.name }) }}</p>
            <p class="text-muted-foreground">{{ $t('accounts.providerPhoneId') }}: <code>{{ result.account.phone_id }}</code></p>
          </div>
        </div>

        <div v-if="result.webhook_registered" class="flex items-start gap-3 rounded-lg border p-3 text-sm">
          <CheckCircle2 class="h-5 w-5 shrink-0 text-emerald-500" />
          <p>{{ $t('accounts.providerWebhookOk') }}</p>
        </div>
        <div v-else class="space-y-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm">
          <div class="flex items-start gap-3">
            <AlertTriangle class="h-5 w-5 shrink-0 text-amber-500" />
            <div>
              <p class="font-medium">{{ $t('accounts.providerWebhookManualTitle') }}</p>
              <p class="text-muted-foreground">{{ result.webhook_error }}</p>
            </div>
          </div>
          <p class="text-muted-foreground">{{ $t('accounts.providerWebhookManualSteps') }}</p>
          <div v-for="row in [
            { label: $t('accounts.webhookUrl'), value: result.webhook_url },
            { label: $t('accounts.providerHeaderName'), value: result.webhook_header },
            { label: $t('accounts.providerHeaderValue'), value: result.webhook_secret },
          ]" :key="row.label" class="space-y-1">
            <p class="text-xs font-medium">{{ row.label }}</p>
            <div class="flex items-center gap-2">
              <code class="flex-1 truncate rounded bg-muted px-2 py-1.5 text-xs">{{ row.value }}</code>
              <Button type="button" variant="ghost" size="icon-sm" :aria-label="$t('common.copy')" @click="copy(row.value)">
                <Copy class="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button type="button" @click="open = false">{{ $t('common.done') }}</Button>
        </DialogFooter>
      </div>
    </DialogContent>
  </Dialog>
</template>
