<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import WhatsAppTemplatePreview from './WhatsAppTemplatePreview.vue'
import { templateAIService, type TemplateAIDraft, type TemplateAIPrompt, type TemplateAIRequest } from '@/services/api'
import { getErrorMessage } from '@/lib/api-utils'
import { formatDate } from '@/lib/utils'
import { toast } from 'vue-sonner'
import { Sparkles, History, Loader2, MousePointerClick, Reply, AlertTriangle, Check, Wand2 } from 'lucide-vue-next'

const props = defineProps<{
  languages: Array<{ code: string; name: string }>
  defaultLanguage?: string
  defaultCategory?: string
}>()
const open = defineModel<boolean>('open', { default: false })
const emit = defineEmits<{ apply: [draft: TemplateAIDraft] }>()

const { t } = useI18n()
const MAX_PROMPT = 1024

const form = ref<TemplateAIRequest>({
  prompt: '',
  category: 'AUTO',
  language: 'en',
  style: 'normal',
  optimize_for: 'click',
  header_type: 'AUTO',
  variations: 2,
})

watch(open, (isOpen) => {
  if (isOpen) {
    if (props.defaultLanguage) form.value.language = props.defaultLanguage
    if (props.defaultCategory === 'MARKETING' || props.defaultCategory === 'UTILITY') form.value.category = props.defaultCategory
  }
})

const styles = computed(() => [
  { value: 'normal', emoji: '😐', label: t('templateAI.styleNormal') },
  { value: 'poetic', emoji: '✍️', label: t('templateAI.stylePoetic') },
  { value: 'exciting', emoji: '🤩', label: t('templateAI.styleExciting') },
  { value: 'funny', emoji: '😜', label: t('templateAI.styleFunny') },
] as const)

const examples = computed(() => [
  t('templateAI.example1'),
  t('templateAI.example2'),
  t('templateAI.example3'),
  t('templateAI.example4'),
])

// Previous prompts
const history = ref<TemplateAIPrompt[]>([])
const historyOpen = ref(false)
const historyLoading = ref(false)
async function loadHistory() {
  historyLoading.value = true
  try {
    history.value = (await templateAIService.prompts()).data.data.prompts || []
  } catch {
    history.value = []
  } finally {
    historyLoading.value = false
  }
}
watch(historyOpen, (v) => { if (v) loadHistory() })

function usePrevious(p: TemplateAIPrompt) {
  form.value = {
    ...form.value,
    prompt: p.prompt,
    category: p.category || 'AUTO',
    language: p.language || form.value.language,
    style: (p.style as TemplateAIRequest['style']) || 'normal',
    optimize_for: (p.optimize_for as TemplateAIRequest['optimize_for']) || 'click',
    header_type: p.header_type || 'AUTO',
  }
  historyOpen.value = false
}

// Generation
const isGenerating = ref(false)
const drafts = ref<TemplateAIDraft[]>([])
const modelUsed = ref('')

async function generate() {
  if (!form.value.prompt.trim()) {
    toast.error(t('templateAI.promptRequired'))
    return
  }
  isGenerating.value = true
  drafts.value = []
  try {
    const res = await templateAIService.generate({ ...form.value, variations: Number(form.value.variations) })
    drafts.value = res.data.data.variations
    modelUsed.value = res.data.data.model
  } catch (e) {
    toast.error(getErrorMessage(e, t('templateAI.generateFailed')))
  } finally {
    isGenerating.value = false
  }
}

function apply(d: TemplateAIDraft) {
  emit('apply', d)
  open.value = false
  toast.success(t('templateAI.applied'))
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="max-w-6xl w-[95vw] max-h-[92vh] overflow-y-auto">
      <DialogHeader>
        <DialogTitle class="flex items-center gap-2">
          <span class="h-8 w-8 rounded-lg bg-emerald-500/15 flex items-center justify-center"><Sparkles class="h-4 w-4 text-emerald-400" /></span>
          {{ $t('templateAI.title') }}
        </DialogTitle>
        <DialogDescription>{{ $t('templateAI.subtitle') }}</DialogDescription>
      </DialogHeader>

      <div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)]">
        <!-- Brief -->
        <form class="space-y-5" @submit.prevent="generate">
          <div class="grid grid-cols-2 gap-3">
            <div class="space-y-1.5">
              <Label class="text-xs">{{ $t('templates.category', 'Category') }}</Label>
              <Select v-model="form.category">
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="AUTO">{{ $t('templateAI.categoryAuto') }}</SelectItem>
                  <SelectItem value="MARKETING">MARKETING</SelectItem>
                  <SelectItem value="UTILITY">UTILITY</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="space-y-1.5">
              <Label class="text-xs">{{ $t('templates.language', 'Language') }}</Label>
              <Select v-model="form.language">
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="lang in languages" :key="lang.code" :value="lang.code">{{ lang.name }}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="space-y-1.5">
              <Label class="text-xs">{{ $t('templates.headerType', 'Header Type') }}</Label>
              <Select v-model="form.header_type">
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="AUTO">{{ $t('templateAI.headerAuto') }}</SelectItem>
                  <SelectItem value="NONE">{{ $t('templateAI.headerNone') }}</SelectItem>
                  <SelectItem value="TEXT">Text</SelectItem>
                  <SelectItem value="IMAGE">Image</SelectItem>
                  <SelectItem value="VIDEO">Video</SelectItem>
                  <SelectItem value="DOCUMENT">Document</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="space-y-1.5">
              <Label class="text-xs">{{ $t('templateAI.variations') }}</Label>
              <Select :model-value="String(form.variations)" @update:model-value="(v) => (form.variations = Number(v))">
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="n in 3" :key="n" :value="String(n)">{{ n }}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div class="space-y-1.5">
            <div class="flex items-center justify-between">
              <Label>{{ $t('templateAI.promptLabel') }} <span class="text-destructive">*</span></Label>
              <Popover v-model:open="historyOpen">
                <PopoverTrigger as-child>
                  <Button type="button" variant="outline" size="sm"><History class="h-4 w-4 mr-1.5" />{{ $t('templateAI.previousPrompts') }}</Button>
                </PopoverTrigger>
                <PopoverContent align="end" class="w-96 p-2">
                  <div v-if="historyLoading" class="p-3 text-sm text-muted-foreground"><Loader2 class="h-4 w-4 animate-spin inline mr-2" />{{ $t('common.loading') }}</div>
                  <p v-else-if="!history.length" class="p-3 text-sm text-muted-foreground">{{ $t('templateAI.noPreviousPrompts') }}</p>
                  <div v-else class="max-h-72 overflow-y-auto space-y-1">
                    <button
                      v-for="p in history" :key="p.id" type="button"
                      class="w-full text-left rounded-md px-3 py-2 hover:bg-white/[0.06] light:hover:bg-gray-100"
                      @click="usePrevious(p)"
                    >
                      <p class="text-sm line-clamp-2">{{ p.prompt }}</p>
                      <p class="text-[11px] text-muted-foreground mt-0.5">{{ p.category }} · {{ p.language }} · {{ p.style }} · {{ formatDate(p.created_at) }}</p>
                    </button>
                  </div>
                </PopoverContent>
              </Popover>
            </div>
            <Textarea v-model="form.prompt" :rows="5" :maxlength="MAX_PROMPT" :placeholder="$t('templateAI.promptPlaceholder')" />
            <div class="flex items-start justify-between gap-2">
              <div class="flex flex-wrap gap-1.5">
                <button
                  v-for="ex in examples" :key="ex" type="button"
                  class="text-[11px] rounded-full border border-white/[0.1] light:border-gray-200 px-2.5 py-1 text-muted-foreground hover:text-foreground hover:border-emerald-500/50"
                  @click="form.prompt = ex"
                >{{ ex }}</button>
              </div>
              <span class="text-xs text-muted-foreground tabular-nums shrink-0">{{ form.prompt.length }}/{{ MAX_PROMPT }}</span>
            </div>
          </div>

          <div class="space-y-2">
            <Label>{{ $t('templateAI.styleLabel') }}</Label>
            <div class="flex flex-wrap gap-2">
              <button
                v-for="s in styles" :key="s.value" type="button"
                :class="['rounded-lg border px-4 py-2 text-sm transition-colors',
                  form.style === s.value ? 'border-emerald-500 bg-emerald-500/10 text-emerald-300 light:text-emerald-700' : 'border-white/[0.1] light:border-gray-200 hover:border-white/30']"
                @click="form.style = s.value"
              >{{ s.emoji }} {{ s.label }}</button>
            </div>
          </div>

          <div class="space-y-2">
            <Label>{{ $t('templateAI.optimizeLabel') }}</Label>
            <div class="flex flex-wrap gap-2">
              <button
                v-for="o in [{ value: 'click', icon: MousePointerClick, label: $t('templateAI.clickRate') }, { value: 'reply', icon: Reply, label: $t('templateAI.replyRate') }]"
                :key="o.value" type="button"
                :class="['rounded-lg border px-4 py-2 text-sm flex items-center gap-2 transition-colors',
                  form.optimize_for === o.value ? 'border-emerald-500 bg-emerald-500/10 text-emerald-300 light:text-emerald-700' : 'border-white/[0.1] light:border-gray-200 hover:border-white/30']"
                @click="form.optimize_for = o.value as 'click' | 'reply'"
              ><component :is="o.icon" class="h-4 w-4" />{{ o.label }}</button>
            </div>
          </div>

          <Button type="submit" class="w-full" :disabled="isGenerating || !form.prompt.trim()">
            <Loader2 v-if="isGenerating" class="h-4 w-4 mr-2 animate-spin" />
            <Wand2 v-else class="h-4 w-4 mr-2" />
            {{ isGenerating ? $t('templateAI.generating') : drafts.length ? $t('templateAI.regenerate') : $t('templateAI.generate') }}
          </Button>
        </form>

        <!-- Results -->
        <div class="space-y-4 min-w-0">
          <div v-if="isGenerating" class="space-y-4">
            <Skeleton v-for="n in form.variations" :key="n" class="h-56 rounded-xl" />
          </div>
          <div v-else-if="!drafts.length" class="h-full min-h-[320px] rounded-xl border border-dashed border-white/[0.1] light:border-gray-300 flex flex-col items-center justify-center text-center p-8 gap-2">
            <Sparkles class="h-8 w-8 text-emerald-400" />
            <p class="font-medium">{{ $t('templateAI.emptyTitle') }}</p>
            <p class="text-sm text-muted-foreground max-w-sm">{{ $t('templateAI.emptyDesc') }}</p>
          </div>
          <template v-else>
            <div v-for="(d, i) in drafts" :key="i" class="rounded-xl border border-white/[0.08] light:border-gray-200 p-4 space-y-3">
              <div class="flex items-center justify-between gap-2">
                <div class="min-w-0">
                  <p class="text-sm font-medium truncate">{{ $t('templateAI.variationN', { n: i + 1 }) }} · <span class="font-mono text-xs text-muted-foreground">{{ d.name }}</span></p>
                  <div class="flex gap-1.5 mt-1">
                    <Badge variant="secondary">{{ d.category }}</Badge>
                    <Badge variant="secondary">{{ d.language }}</Badge>
                    <Badge v-if="d.header_type !== 'NONE'" variant="secondary">{{ d.header_type }}</Badge>
                  </div>
                </div>
                <Button size="sm" @click="apply(d)"><Check class="h-4 w-4 mr-1" />{{ $t('templateAI.useThis') }}</Button>
              </div>
              <WhatsAppTemplatePreview :draft="d" />
              <ul v-if="d.warnings.length" class="space-y-1">
                <li v-for="w in d.warnings" :key="w" class="flex gap-1.5 text-xs text-amber-400 light:text-amber-700">
                  <AlertTriangle class="h-3.5 w-3.5 shrink-0 mt-0.5" />{{ w }}
                </li>
              </ul>
            </div>
            <p class="text-[11px] text-muted-foreground">{{ $t('templateAI.disclaimer', { model: modelUsed }) }}</p>
          </template>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
