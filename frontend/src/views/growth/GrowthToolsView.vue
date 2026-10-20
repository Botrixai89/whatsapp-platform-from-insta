<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import { PageHeader, DateRangePicker } from '@/components/shared'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { growthService, type TrackedLink, type AdLeadRow } from '@/services/api'
import { useDateRange } from '@/composables/useDateRange'
import { getErrorMessage } from '@/lib/api-utils'
import { formatDate } from '@/lib/utils'
import { toast } from 'vue-sonner'
import { Rocket, Copy, Download, Trash2, ExternalLink, Loader2, ImagePlus } from 'lucide-vue-next'

const { t } = useI18n()

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(t('common.copied'))
  } catch {
    toast.error(t('common.clipboardFailed'))
  }
}

// --- WhatsApp link + QR ---------------------------------------------------
const phone = ref('')
const prefill = ref('')
const qrDataUrl = ref('')
// A 10-digit Indian mobile number without country code gets 91 added, since
// wa.me links only work with the full international number.
const cleanPhone = computed(() => {
  const digits = phone.value.replace(/\D/g, '')
  return /^[6-9]\d{9}$/.test(digits) ? '91' + digits : digits
})
const addedCountryCode = computed(() => /^[6-9]\d{9}$/.test(phone.value.replace(/\D/g, '')))
const waLink = computed(() => {
  if (cleanPhone.value.length < 11) return ''
  const text = prefill.value.trim()
  return `https://wa.me/${cleanPhone.value}${text ? `?text=${encodeURIComponent(text)}` : ''}`
})

// Brand styling, remembered in this browser
const QR_LOGO_KEY = 'growth_qr_logo'
const QR_COLOR_KEY = 'growth_qr_color'
function readStored(key: string) {
  try { return localStorage.getItem(key) || '' } catch { return '' }
}
function store(key: string, value: string) {
  try { if (value) localStorage.setItem(key, value); else localStorage.removeItem(key) } catch { /* storage full or blocked */ }
}
const qrLogo = ref(readStored(QR_LOGO_KEY))
const qrColor = ref(readStored(QR_COLOR_KEY) || '#000000')
const logoInput = ref<HTMLInputElement | null>(null)

// Phones need strong contrast against the white background to scan
const qrColorTooLight = computed(() => {
  const m = /^#([0-9a-f]{6})$/i.exec(qrColor.value)
  if (!m) return true
  const n = parseInt(m[1], 16)
  const lum = (0.2126 * ((n >> 16) & 255) + 0.7152 * ((n >> 8) & 255) + 0.0722 * (n & 255)) / 255
  return lum > 0.55
})

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = reject
    img.src = src
  })
}

// Shrink the uploaded logo so it is small enough to remember and quick to draw
async function onLogoFile(ev: Event) {
  const file = (ev.target as HTMLInputElement).files?.[0]
  if (logoInput.value) logoInput.value.value = ''
  if (!file) return
  if (!file.type.startsWith('image/') || file.size > 5 * 1024 * 1024) {
    toast.error(t('growth.logoInvalid'))
    return
  }
  try {
    const dataUrl = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(String(reader.result))
      reader.onerror = reject
      reader.readAsDataURL(file)
    })
    const img = await loadImage(dataUrl)
    const max = 256
    const w = img.naturalWidth || max
    const h = img.naturalHeight || max
    const scale = Math.min(1, max / Math.max(w, h))
    const canvas = document.createElement('canvas')
    canvas.width = Math.max(1, Math.round(w * scale))
    canvas.height = Math.max(1, Math.round(h * scale))
    canvas.getContext('2d')!.drawImage(img, 0, 0, canvas.width, canvas.height)
    qrLogo.value = canvas.toDataURL('image/png')
    store(QR_LOGO_KEY, qrLogo.value)
  } catch {
    toast.error(t('growth.logoInvalid'))
  }
}
function removeLogo() {
  qrLogo.value = ''
  store(QR_LOGO_KEY, '')
}

async function renderQR() {
  if (!waLink.value) {
    qrDataUrl.value = ''
    return
  }
  const size = 1024
  const dark = qrColorTooLight.value ? '#000000' : qrColor.value
  // High error correction keeps the code scannable with the centre covered by the logo
  const base = await QRCode.toDataURL(waLink.value, {
    width: size, margin: 2, errorCorrectionLevel: qrLogo.value ? 'H' : 'M',
    color: { dark, light: '#ffffff' },
  })
  if (!qrLogo.value) {
    qrDataUrl.value = base
    return
  }
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = size
  const ctx = canvas.getContext('2d')!
  ctx.drawImage(await loadImage(base), 0, 0, size, size)
  const logo = await loadImage(qrLogo.value)
  const box = Math.round(size * 0.24) // well under the ~30% that level H can recover
  const pad = Math.round(box * 0.12)
  const x = (size - box) / 2
  ctx.fillStyle = '#ffffff'
  ctx.beginPath()
  ctx.roundRect(x - pad, x - pad, box + pad * 2, box + pad * 2, Math.round(box * 0.18))
  ctx.fill()
  const scale = Math.min(box / logo.width, box / logo.height)
  const lw = logo.width * scale
  const lh = logo.height * scale
  ctx.drawImage(logo, (size - lw) / 2, (size - lh) / 2, lw, lh)
  qrDataUrl.value = canvas.toDataURL('image/png')
}
watch([waLink, qrLogo, qrColor], () => {
  store(QR_COLOR_KEY, qrColor.value === '#000000' ? '' : qrColor.value)
  renderQR().catch(() => { qrDataUrl.value = '' })
}, { immediate: true })

function downloadQR() {
  const a = document.createElement('a')
  a.href = qrDataUrl.value
  a.download = `whatsapp-qr-${cleanPhone.value}.png`
  a.click()
}

// --- Website chat button ---------------------------------------------------
const widget = ref({ position: 'right', color: '#25D366', greeting: 'Hi! How can we help you?', label: 'Chat with us' })
const snippet = computed(() => {
  if (!waLink.value) return ''
  const side = widget.value.position === 'left' ? 'left' : 'right'
  const color = /^#[0-9a-fA-F]{3,8}$/.test(widget.value.color) ? widget.value.color : '#25D366'
  const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
  return `<!-- WhatsApp chat button -->
<a href="${waLink.value}" target="_blank" rel="noopener" aria-label="${esc(widget.value.label)}"
  style="position:fixed;${side}:20px;bottom:20px;z-index:2147483000;display:flex;align-items:center;gap:8px;
  padding:12px 16px;border-radius:999px;background:${color};color:#fff;font:600 14px/1 system-ui,sans-serif;
  text-decoration:none;box-shadow:0 6px 20px rgba(0,0,0,.25)">
  <svg width="22" height="22" viewBox="0 0 24 24" fill="#fff" aria-hidden="true"><path d="M12 2a10 10 0 0 0-8.6 15.1L2 22l5-1.3A10 10 0 1 0 12 2Zm0 18.2a8.2 8.2 0 0 1-4.2-1.2l-.3-.2-3 .8.8-2.9-.2-.3A8.2 8.2 0 1 1 12 20.2Zm4.5-6.1c-.2-.1-1.5-.7-1.7-.8s-.4-.1-.6.1-.7.8-.8 1-.3.2-.5.1a6.7 6.7 0 0 1-3.3-2.9c-.3-.4.2-.4.7-1.4.1-.2 0-.3 0-.4l-.8-1.8c-.2-.5-.4-.4-.6-.4h-.5a1 1 0 0 0-.7.3 3 3 0 0 0-.9 2.2 5.2 5.2 0 0 0 1.1 2.7 11.8 11.8 0 0 0 4.5 4c1.7.7 2.3.8 3.2.6a2.7 2.7 0 0 0 1.8-1.2 2.2 2.2 0 0 0 .2-1.2c-.1-.1-.3-.2-.5-.3Z"/></svg>
  ${esc(widget.value.label)}
</a>${widget.value.greeting.trim() ? `
<div style="position:fixed;${side}:20px;bottom:76px;z-index:2147483000;max-width:240px;padding:10px 12px;
  border-radius:12px;background:#fff;color:#111;font:14px/1.4 system-ui,sans-serif;box-shadow:0 6px 20px rgba(0,0,0,.18)">
  ${esc(widget.value.greeting.trim())}
</div>` : ''}`
})

// --- Tracked links ---------------------------------------------------------
const links = ref<TrackedLink[]>([])
const newLink = ref({ target_url: '', title: '' })
const isCreatingLink = ref(false)
async function loadLinks() {
  try {
    links.value = (await growthService.links()).data.data.links
  } catch (e) {
    toast.error(getErrorMessage(e, t('growth.linksLoadFailed')))
  }
}
async function createLink() {
  if (!newLink.value.target_url.trim()) return
  isCreatingLink.value = true
  try {
    const created = (await growthService.createLink({ target_url: newLink.value.target_url.trim(), title: newLink.value.title.trim() })).data.data
    links.value.unshift(created)
    newLink.value = { target_url: '', title: '' }
    copy(created.short_url)
  } catch (e) {
    toast.error(getErrorMessage(e, t('growth.linkCreateFailed')))
  } finally {
    isCreatingLink.value = false
  }
}
async function deleteLink(link: TrackedLink) {
  try {
    await growthService.deleteLink(link.id)
    links.value = links.value.filter(l => l.id !== link.id)
  } catch (e) {
    toast.error(getErrorMessage(e, t('growth.linkDeleteFailed')))
  }
}
const isPrivateHost = computed(() => /\/\/(localhost|127\.|192\.168\.|10\.)/.test(links.value[0]?.short_url || window.location.origin))

// --- Ad leads ----------------------------------------------------------------
const ads = ref<AdLeadRow[]>([])
const totalLeads = ref(0)
const isLoadingAds = ref(false)
const { selectedRange, customDateRange, isDatePickerOpen, dateRange, formatDateRangeDisplay, applyCustomRange: baseApply } =
  useDateRange({ defaultPreset: '30days', storageKey: 'growth_ads' })
async function loadAds() {
  isLoadingAds.value = true
  try {
    const { from, to } = dateRange.value
    const data = (await growthService.ads({ from, to })).data.data
    ads.value = data.ads
    totalLeads.value = data.total_leads
  } catch (e) {
    toast.error(getErrorMessage(e, t('growth.adsLoadFailed')))
  } finally {
    isLoadingAds.value = false
  }
}
function applyCustomRange() {
  baseApply()
  loadAds()
}
watch(selectedRange, v => { if (v !== 'custom') loadAds() })

onMounted(() => {
  loadLinks()
  loadAds()
})
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('growth.title')" :description="$t('growth.subtitle')" :icon="Rocket" />

    <ScrollArea class="flex-1">
      <div class="p-6">
        <Tabs default-value="link" class="w-full">
          <TabsList class="mb-6">
            <TabsTrigger value="link">{{ $t('growth.tabLink') }}</TabsTrigger>
            <TabsTrigger value="widget">{{ $t('growth.tabWidget') }}</TabsTrigger>
            <TabsTrigger value="tracked">{{ $t('growth.tabTracked') }}</TabsTrigger>
            <TabsTrigger value="ads">{{ $t('growth.tabAds') }}</TabsTrigger>
          </TabsList>

          <!-- WhatsApp link + QR -->
          <TabsContent value="link">
            <div class="grid gap-4 lg:grid-cols-[1fr_320px]">
              <Card>
                <CardHeader>
                  <CardTitle>{{ $t('growth.linkTitle') }}</CardTitle>
                  <CardDescription>{{ $t('growth.linkDesc') }}</CardDescription>
                </CardHeader>
                <CardContent class="space-y-4">
                  <div class="space-y-2">
                    <Label for="g-phone">{{ $t('growth.phone') }}</Label>
                    <Input id="g-phone" v-model="phone" inputmode="tel" placeholder="919876543210" />
                    <p v-if="addedCountryCode" class="text-xs text-emerald-500">{{ $t('growth.addedCountryCode', { n: cleanPhone }) }}</p>
                    <p v-else-if="phone && cleanPhone.length < 11" class="text-xs text-amber-500">{{ $t('growth.needCountryCode') }}</p>
                    <p v-else class="text-xs text-muted-foreground">{{ $t('growth.phoneHint') }}</p>
                  </div>
                  <div class="space-y-2">
                    <Label for="g-text">{{ $t('growth.prefill') }}</Label>
                    <Textarea id="g-text" v-model="prefill" :rows="3" :placeholder="$t('growth.prefillPlaceholder')" />
                    <p class="text-xs text-muted-foreground">{{ $t('growth.prefillHint') }}</p>
                  </div>
                  <div v-if="waLink" class="flex items-center gap-2">
                    <code class="flex-1 truncate rounded bg-muted px-3 py-2 text-xs">{{ waLink }}</code>
                    <Button variant="outline" size="sm" @click="copy(waLink)"><Copy class="h-4 w-4" />{{ $t('common.copy') }}</Button>
                    <a :href="waLink" target="_blank" rel="noopener"><Button variant="ghost" size="icon-sm" :aria-label="$t('growth.open')"><ExternalLink class="h-4 w-4" /></Button></a>
                  </div>
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle>{{ $t('growth.qrTitle') }}</CardTitle>
                  <CardDescription>{{ $t('growth.qrDesc') }}</CardDescription>
                </CardHeader>
                <CardContent class="flex flex-col items-center gap-3">
                  <div class="flex aspect-square w-full max-w-[240px] items-center justify-center rounded-lg bg-white p-2">
                    <img v-if="qrDataUrl" :src="qrDataUrl" :alt="$t('growth.qrTitle')" class="h-full w-full" />
                    <p v-else class="px-4 text-center text-xs text-gray-500">{{ $t('growth.qrEmpty') }}</p>
                  </div>
                  <!-- Brand styling -->
                  <div class="w-full space-y-3 border-t border-white/[0.08] pt-3 light:border-gray-200">
                    <div class="flex items-center gap-2">
                      <input ref="logoInput" type="file" accept="image/png,image/jpeg,image/webp,image/svg+xml" class="hidden" @change="onLogoFile" />
                      <div v-if="qrLogo" class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border bg-white p-1">
                        <img :src="qrLogo" alt="" class="max-h-full max-w-full" />
                      </div>
                      <Button variant="outline" size="sm" class="flex-1" @click="logoInput?.click()">
                        <ImagePlus class="h-4 w-4" />{{ qrLogo ? $t('growth.changeLogo') : $t('growth.addLogo') }}
                      </Button>
                      <Button v-if="qrLogo" variant="ghost" size="icon-sm" :aria-label="$t('growth.removeLogo')" @click="removeLogo">
                        <Trash2 class="h-4 w-4" />
                      </Button>
                    </div>
                    <div class="flex items-center gap-2">
                      <Label for="qr-color" class="text-xs">{{ $t('growth.qrColor') }}</Label>
                      <input id="qr-color" v-model="qrColor" type="color" class="ml-auto h-8 w-10 cursor-pointer rounded border bg-transparent" />
                      <Button v-if="qrColor !== '#000000'" variant="ghost" size="sm" class="h-8 px-2 text-xs" @click="qrColor = '#000000'">{{ $t('growth.resetColor') }}</Button>
                    </div>
                    <p v-if="qrColorTooLight" class="text-xs text-amber-500">{{ $t('growth.colorTooLight') }}</p>
                  </div>
                  <Button :disabled="!qrDataUrl" size="sm" class="w-full" @click="downloadQR"><Download class="h-4 w-4" />{{ $t('growth.downloadQr') }}</Button>
                </CardContent>
              </Card>
            </div>
          </TabsContent>

          <!-- Website chat button -->
          <TabsContent value="widget">
            <Card>
              <CardHeader>
                <CardTitle>{{ $t('growth.widgetTitle') }}</CardTitle>
                <CardDescription>{{ $t('growth.widgetDesc') }}</CardDescription>
              </CardHeader>
              <CardContent class="space-y-4">
                <p v-if="!waLink" class="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">{{ $t('growth.widgetNeedsNumber') }}</p>
                <template v-else>
                  <div class="grid gap-4 md:grid-cols-4">
                    <div class="space-y-2">
                      <Label>{{ $t('growth.position') }}</Label>
                      <Select v-model="widget.position">
                        <SelectTrigger><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="right">{{ $t('growth.bottomRight') }}</SelectItem>
                          <SelectItem value="left">{{ $t('growth.bottomLeft') }}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div class="space-y-2">
                      <Label for="g-color">{{ $t('growth.color') }}</Label>
                      <div class="flex gap-2">
                        <input id="g-color" v-model="widget.color" type="color" class="h-9 w-12 cursor-pointer rounded border bg-transparent" />
                        <Input v-model="widget.color" class="font-mono" />
                      </div>
                    </div>
                    <div class="space-y-2">
                      <Label for="g-label">{{ $t('growth.buttonText') }}</Label>
                      <Input id="g-label" v-model="widget.label" />
                    </div>
                    <div class="space-y-2">
                      <Label for="g-greet">{{ $t('growth.greeting') }}</Label>
                      <Input id="g-greet" v-model="widget.greeting" />
                    </div>
                  </div>
                  <div class="space-y-2">
                    <div class="flex items-center justify-between">
                      <Label>{{ $t('growth.snippet') }}</Label>
                      <Button variant="outline" size="sm" @click="copy(snippet)"><Copy class="h-4 w-4" />{{ $t('growth.copySnippet') }}</Button>
                    </div>
                    <pre class="max-h-56 overflow-auto rounded-lg bg-muted p-3 text-xs leading-relaxed"><code>{{ snippet }}</code></pre>
                    <p class="text-xs text-muted-foreground">{{ $t('growth.snippetHint') }}</p>
                  </div>
                </template>
              </CardContent>
            </Card>
          </TabsContent>

          <!-- Tracked links -->
          <TabsContent value="tracked">
            <Card>
              <CardHeader>
                <CardTitle>{{ $t('growth.trackedTitle') }}</CardTitle>
                <CardDescription>{{ $t('growth.trackedDesc') }}</CardDescription>
              </CardHeader>
              <CardContent class="space-y-4">
                <form class="grid gap-3 md:grid-cols-[2fr_1fr_auto]" @submit.prevent="createLink">
                  <Input v-model="newLink.target_url" type="url" :placeholder="$t('growth.targetPlaceholder')" required />
                  <Input v-model="newLink.title" :placeholder="$t('growth.titlePlaceholder')" />
                  <Button type="submit" :disabled="isCreatingLink">
                    <Loader2 v-if="isCreatingLink" class="h-4 w-4 animate-spin" />{{ $t('growth.createLink') }}
                  </Button>
                </form>
                <p v-if="isPrivateHost" class="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs">{{ $t('growth.privateHostWarning') }}</p>
                <p v-if="!links.length" class="py-6 text-center text-sm text-muted-foreground">{{ $t('growth.noLinks') }}</p>
                <ul v-else class="divide-y divide-white/[0.06] light:divide-gray-100">
                  <li v-for="l in links" :key="l.id" class="flex items-center gap-3 py-3">
                    <div class="min-w-0 flex-1">
                      <p class="truncate text-sm font-medium">{{ l.title || l.target_url }}</p>
                      <p class="truncate text-xs text-muted-foreground">{{ l.short_url }} → {{ l.target_url }}</p>
                    </div>
                    <div class="text-right">
                      <p class="text-sm font-semibold tabular-nums">{{ l.clicks }}</p>
                      <p class="text-[11px] text-muted-foreground">{{ $t('growth.clicks') }}</p>
                    </div>
                    <Button variant="ghost" size="icon-sm" :aria-label="$t('common.copy')" @click="copy(l.short_url)"><Copy class="h-4 w-4" /></Button>
                    <Button variant="ghost" size="icon-sm" :aria-label="$t('common.delete')" @click="deleteLink(l)"><Trash2 class="h-4 w-4 text-destructive" /></Button>
                  </li>
                </ul>
              </CardContent>
            </Card>
          </TabsContent>

          <!-- Click-to-WhatsApp ad leads -->
          <TabsContent value="ads">
            <Card>
              <CardHeader class="flex flex-row items-start justify-between gap-4 space-y-0">
                <div>
                  <CardTitle>{{ $t('growth.adsTitle') }}</CardTitle>
                  <CardDescription class="mt-1">{{ $t('growth.adsDesc', { n: totalLeads }) }}</CardDescription>
                </div>
                <DateRangePicker
                  v-model:selected-range="selectedRange"
                  v-model:custom-date-range="customDateRange"
                  v-model:is-date-picker-open="isDatePickerOpen"
                  :format-date-range-display="formatDateRangeDisplay"
                  @apply-custom="applyCustomRange"
                />
              </CardHeader>
              <CardContent>
                <p v-if="!isLoadingAds && !ads.length" class="py-6 text-center text-sm text-muted-foreground">{{ $t('growth.noAds') }}</p>
                <table v-else class="w-full text-sm">
                  <thead class="text-left text-xs text-muted-foreground">
                    <tr>
                      <th class="pb-2 font-medium">{{ $t('growth.ad') }}</th>
                      <th class="pb-2 text-right font-medium">{{ $t('growth.leads') }}</th>
                      <th class="pb-2 text-right font-medium">{{ $t('growth.chatsStarted') }}</th>
                      <th class="pb-2 text-right font-medium">{{ $t('growth.lastLead') }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="ad in ads" :key="ad.source_id" class="border-t border-white/[0.06] light:border-gray-100">
                      <td class="py-2.5 pr-3">
                        <p class="font-medium">{{ ad.headline || $t('growth.untitledAd') }}</p>
                        <p class="text-xs text-muted-foreground">{{ ad.source_type === 'post' ? $t('growth.post') : $t('growth.adLabel') }} · {{ ad.source_id }}</p>
                      </td>
                      <td class="py-2.5 text-right tabular-nums">{{ ad.leads }}</td>
                      <td class="py-2.5 text-right tabular-nums">{{ ad.clicks }}</td>
                      <td class="py-2.5 text-right text-muted-foreground">{{ formatDate(ad.last_at) }}</td>
                    </tr>
                  </tbody>
                </table>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>
    </ScrollArea>
  </div>
</template>
