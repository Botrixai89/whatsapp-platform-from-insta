<script setup lang="ts">
import { ref, computed } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { toast } from 'vue-sonner'
import { Copy, KeyRound } from 'lucide-vue-next'

/** Shows a ready-to-run API request that sends this template. */
const open = defineModel<boolean>('open', { required: true })
const props = defineProps<{
  template: {
    name: string
    whatsapp_account?: string
    header_type?: string
    header_content?: string
    body_content?: string
    buttons?: Array<{ type: string; url?: string }>
    sample_values?: Array<{ component: string; index: number; value: string }>
  } | null
}>()
const { t } = useI18n()

const lang = ref<'curl' | 'node' | 'python'>('curl')
const endpoint = `${window.location.origin}/api/messages/template`

function varsOf(text = '') {
  return [...text.matchAll(/\{\{([^}]+)\}\}/g)].map(m => m[1].trim())
}

const payload = computed(() => {
  const tpl = props.template
  if (!tpl) return {}
  const sample = (component: string, i: number) =>
    tpl.sample_values?.find(s => s.component === component && s.index === i)?.value || `value${i}`
  const body: Record<string, any> = {
    phone_number: '919876543210',
    template_name: tpl.name,
  }
  if (tpl.whatsapp_account) body.account_name = tpl.whatsapp_account
  const bodyVars = varsOf(tpl.body_content)
  if (bodyVars.length) body.template_params = Object.fromEntries(bodyVars.map((v, i) => [v, sample('body', i + 1)]))
  const headerVars = tpl.header_type === 'TEXT' ? varsOf(tpl.header_content) : []
  if (headerVars.length) body.header_params = { [headerVars[0]]: sample('header', 1) }
  if (['IMAGE', 'VIDEO', 'DOCUMENT'].includes(tpl.header_type || '')) body.header_media_url = 'https://example.com/media-file'
  const urlButtons = (tpl.buttons || []).map((b, i) => ({ b, i })).filter(({ b }) => b.type === 'URL' && b.url?.includes('{{'))
  if (urlButtons.length) body.button_params = Object.fromEntries(urlButtons.map(({ i }) => [String(i), 'dynamic-path']))
  return body
})

const snippet = computed(() => {
  const json = JSON.stringify(payload.value, null, 2)
  if (lang.value === 'node') {
    return `const res = await fetch('${endpoint}', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json', 'X-API-Key': 'YOUR_API_KEY' },
  body: JSON.stringify(${json.replace(/\n/g, '\n  ')}),
})
console.log(await res.json())`
  }
  if (lang.value === 'python') {
    return `import requests

res = requests.post(
    "${endpoint}",
    headers={"X-API-Key": "YOUR_API_KEY"},
    json=${json.replace(/\n/g, '\n    ').replace(/: true/g, ': True').replace(/: false/g, ': False')},
)
print(res.json())`
  }
  return `curl -X POST '${endpoint}' \\
  -H 'Content-Type: application/json' \\
  -H 'X-API-Key: YOUR_API_KEY' \\
  -d '${json.replace(/'/g, "'\\''")}'`
})

async function copy() {
  try {
    await navigator.clipboard.writeText(snippet.value)
    toast.success(t('common.copied'))
  } catch {
    toast.error(t('common.copyFailed'))
  }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>{{ $t('templates.apiTitle') }}</DialogTitle>
        <DialogDescription>{{ $t('templates.apiDesc', { name: template?.name }) }}</DialogDescription>
      </DialogHeader>
      <div class="flex items-center justify-between gap-2">
        <div class="flex gap-1">
          <Button v-for="l in (['curl', 'node', 'python'] as const)" :key="l" size="sm" :variant="lang === l ? 'secondary' : 'ghost'" class="h-7 px-3 text-xs" @click="lang = l">
            {{ l === 'curl' ? 'cURL' : l === 'node' ? 'Node.js' : 'Python' }}
          </Button>
        </div>
        <Button size="sm" variant="outline" class="h-7" @click="copy"><Copy class="h-3.5 w-3.5" />{{ $t('common.copy') }}</Button>
      </div>
      <pre class="max-h-[50vh] overflow-auto rounded-lg bg-black/40 p-4 font-mono text-xs leading-relaxed text-white/85 light:bg-gray-950 light:text-gray-100">{{ snippet }}</pre>
      <p class="flex items-center gap-2 text-xs text-muted-foreground">
        <KeyRound class="h-3.5 w-3.5 shrink-0" />
        <span>{{ $t('templates.apiKeyHint') }} <RouterLink to="/settings/api-keys" class="text-primary underline-offset-2 hover:underline" @click="open = false">{{ $t('templates.apiKeysLink') }}</RouterLink></span>
      </p>
    </DialogContent>
  </Dialog>
</template>
