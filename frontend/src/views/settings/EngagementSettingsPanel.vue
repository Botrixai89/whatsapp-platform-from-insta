<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Skeleton } from '@/components/ui/skeleton'
import { engagementService, type EngagementSettings } from '@/services/api'
import { getErrorMessage } from '@/lib/api-utils'
import { toast } from 'vue-sonner'
import { Loader2 } from 'lucide-vue-next'

/** Opt-out keywords (STOP/START) and the CSAT survey sent when a chat is closed. */
const { t } = useI18n()
const settings = ref<EngagementSettings | null>(null)
// Keywords are edited as comma-separated text
const optOutText = ref('')
const optInText = ref('')
const isSaving = ref(false)

const toList = (s: string) => s.split(',').map(k => k.trim()).filter(Boolean)

onMounted(async () => {
  try {
    settings.value = (await engagementService.get()).data.data
    optOutText.value = settings.value.opt_out.opt_out_keywords.join(', ')
    optInText.value = settings.value.opt_out.opt_in_keywords.join(', ')
  } catch (e) {
    toast.error(getErrorMessage(e, t('engagement.loadFailed')))
  }
})

async function save() {
  if (!settings.value) return
  isSaving.value = true
  try {
    const payload: EngagementSettings = {
      ...settings.value,
      opt_out: { ...settings.value.opt_out, opt_out_keywords: toList(optOutText.value), opt_in_keywords: toList(optInText.value) },
    }
    settings.value = (await engagementService.update(payload)).data.data
    optOutText.value = settings.value.opt_out.opt_out_keywords.join(', ')
    optInText.value = settings.value.opt_out.opt_in_keywords.join(', ')
    toast.success(t('common.savedSuccess', { resource: t('engagement.title') }))
  } catch (e) {
    toast.error(getErrorMessage(e, t('engagement.saveFailed')))
  } finally {
    isSaving.value = false
  }
}
</script>

<template>
  <div v-if="!settings" class="space-y-4">
    <Skeleton class="h-64 rounded-xl" />
    <Skeleton class="h-48 rounded-xl" />
  </div>
  <div v-else class="space-y-4">
    <Card>
      <CardHeader class="flex flex-row items-start justify-between gap-4 space-y-0">
        <div>
          <CardTitle>{{ $t('engagement.optOutTitle') }}</CardTitle>
          <CardDescription class="mt-1">{{ $t('engagement.optOutDesc') }}</CardDescription>
        </div>
        <Switch v-model:checked="settings.opt_out.enabled" :aria-label="$t('engagement.optOutTitle')" />
      </CardHeader>
      <CardContent v-if="settings.opt_out.enabled" class="space-y-4">
        <div class="grid gap-4 md:grid-cols-2">
          <div class="space-y-2">
            <Label for="optout-kw">{{ $t('engagement.optOutKeywords') }}</Label>
            <Input id="optout-kw" v-model="optOutText" placeholder="STOP, UNSUBSCRIBE" />
          </div>
          <div class="space-y-2">
            <Label for="optin-kw">{{ $t('engagement.optInKeywords') }}</Label>
            <Input id="optin-kw" v-model="optInText" placeholder="START, SUBSCRIBE" />
          </div>
        </div>
        <p class="text-xs text-muted-foreground">{{ $t('engagement.keywordHint') }}</p>
        <div class="space-y-2">
          <Label for="optout-reply">{{ $t('engagement.optOutReply') }}</Label>
          <Textarea id="optout-reply" v-model="settings.opt_out.opt_out_reply" :rows="2" />
        </div>
        <div class="space-y-2">
          <Label for="optin-reply">{{ $t('engagement.optInReply') }}</Label>
          <Textarea id="optin-reply" v-model="settings.opt_out.opt_in_reply" :rows="2" />
        </div>
      </CardContent>
    </Card>

    <Card>
      <CardHeader class="flex flex-row items-start justify-between gap-4 space-y-0">
        <div>
          <CardTitle>{{ $t('engagement.csatTitle') }}</CardTitle>
          <CardDescription class="mt-1">{{ $t('engagement.csatDesc') }}</CardDescription>
        </div>
        <Switch v-model:checked="settings.csat.enabled" :aria-label="$t('engagement.csatTitle')" />
      </CardHeader>
      <CardContent v-if="settings.csat.enabled" class="space-y-4">
        <div class="space-y-2">
          <Label for="csat-q">{{ $t('engagement.csatQuestion') }}</Label>
          <Textarea id="csat-q" v-model="settings.csat.question" :rows="2" />
          <p class="text-xs text-muted-foreground">{{ $t('engagement.csatButtonsHint') }}</p>
        </div>
        <div class="space-y-2">
          <Label for="csat-thanks">{{ $t('engagement.csatThanks') }}</Label>
          <Input id="csat-thanks" v-model="settings.csat.thanks_message" />
        </div>
      </CardContent>
    </Card>

    <div class="flex justify-end">
      <Button :disabled="isSaving" @click="save">
        <Loader2 v-if="isSaving" class="h-4 w-4 animate-spin" />
        {{ $t('common.save') }}
      </Button>
    </div>
  </div>
</template>
