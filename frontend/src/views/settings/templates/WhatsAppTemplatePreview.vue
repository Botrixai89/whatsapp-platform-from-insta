<script setup lang="ts">
import { computed } from 'vue'
import { Image, Video, FileText, ExternalLink, Phone, Copy, Reply, Workflow, PhoneCall, KeyRound } from 'lucide-vue-next'

export interface PreviewDraft {
  header_type: string
  header_content: string
  body_content: string
  footer_content: string
  buttons: Array<{ type: string; text?: string }>
  sample_values: Array<{ component: string; index: number; value: string }>
}

/** WhatsApp-style chat bubble preview of a template. */
const props = defineProps<{
  draft: PreviewDraft
  /** Local URL of the header media the user picked (image/video), if any */
  mediaUrl?: string
  /** Text shown when the body is still empty */
  placeholder?: string
}>()

function escapeHtml(s: string) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

/**
 * Renders WhatsApp formatting and swaps variables for their sample values.
 * Samples are keyed by occurrence order within a component, which matches the
 * template editor for both positional ({{1}}) and named ({{name}}) variables.
 */
function render(text: string, component: 'header' | 'body') {
  let occurrence = 0
  let html = escapeHtml(text)
  html = html.replace(/\{\{([^}]+)\}\}/g, (_, name) => {
    occurrence++
    const sample = props.draft.sample_values?.find(s => s.component === component && s.index === occurrence)
    const label = sample?.value?.trim() ? sample.value : `{{${name.trim()}}}`
    return `<span class="rounded bg-emerald-500/20 px-1 text-emerald-200 light:text-emerald-800">${escapeHtml(label)}</span>`
  })
  return html
    .replace(/\*([^*\n]+)\*/g, '<strong>$1</strong>')
    .replace(/(^|\W)_([^_\n]+)_(?=\W|$)/g, '$1<em>$2</em>')
    .replace(/~([^~\n]+)~/g, '<s>$1</s>')
    .replace(/```([^`]+)```/g, '<code class="font-mono text-[12px]">$1</code>')
    .replace(/\n/g, '<br>')
}

const headerType = computed(() => (props.draft.header_type || 'NONE').toUpperCase())
const header = computed(() => headerType.value === 'TEXT' && props.draft.header_content ? render(props.draft.header_content, 'header') : '')
const body = computed(() => props.draft.body_content?.trim() ? render(props.draft.body_content, 'body') : '')
const mediaIcon = computed(() => ({ IMAGE: Image, VIDEO: Video, DOCUMENT: FileText } as Record<string, any>)[headerType.value])
const buttons = computed(() => (props.draft.buttons || []).filter(b => b.text?.trim() || b.type === 'OTP'))
const buttonIcon = (type: string) => ({
  URL: ExternalLink, PHONE_NUMBER: Phone, COPY_CODE: Copy, QUICK_REPLY: Reply,
  FLOW: Workflow, VOICE_CALL: PhoneCall, OTP: KeyRound,
} as Record<string, any>)[type] || Reply
const now = computed(() => new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }))
</script>

<template>
  <div class="rounded-lg bg-[#0b141a] light:bg-[#efeae2] p-3">
    <div class="max-w-[340px] rounded-lg rounded-tl-none bg-[#202c33] light:bg-white shadow text-[13px] leading-relaxed text-white/90 light:text-gray-900 overflow-hidden">
      <template v-if="mediaIcon">
        <img v-if="mediaUrl && headerType === 'IMAGE'" :src="mediaUrl" alt="" class="w-full max-h-48 object-cover" />
        <video v-else-if="mediaUrl && headerType === 'VIDEO'" :src="mediaUrl" class="w-full max-h-48 object-cover" muted />
        <div v-else class="h-32 bg-white/[0.06] light:bg-gray-100 flex items-center justify-center">
          <component :is="mediaIcon" class="h-8 w-8 text-white/40 light:text-gray-400" />
        </div>
      </template>
      <div class="px-3 pt-2 pb-1.5 space-y-1">
        <!-- eslint-disable-next-line vue/no-v-html -->
        <p v-if="header" class="font-semibold break-words" v-html="header" />
        <!-- eslint-disable-next-line vue/no-v-html -->
        <p v-if="body" class="whitespace-normal break-words" v-html="body" />
        <p v-else class="italic text-white/40 light:text-gray-400">{{ placeholder || '…' }}</p>
        <div class="flex items-end justify-between gap-2">
          <p v-if="draft.footer_content" class="text-[11px] text-white/45 light:text-gray-500 break-words">{{ draft.footer_content }}</p>
          <span class="ml-auto text-[10px] text-white/40 light:text-gray-400 shrink-0">{{ now }}</span>
        </div>
      </div>
      <div v-if="buttons.length" class="border-t border-white/[0.08] light:border-gray-200">
        <div
          v-for="(b, i) in buttons" :key="i"
          class="flex items-center justify-center gap-1.5 py-2 text-[13px] text-sky-400 light:text-sky-600 border-t first:border-t-0 border-white/[0.08] light:border-gray-200"
        >
          <component :is="buttonIcon(b.type)" class="h-3.5 w-3.5" />{{ b.text || 'Copy code' }}
        </div>
      </div>
    </div>
  </div>
</template>
