<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useHelpAssistant } from '@/composables/useHelpAssistant'
import { assistantService } from '@/services/api'
import { DESTINATIONS, matchLocally, suggestionsFor, type AssistantDestination, type AssistantFaq } from '@/lib/assistant'
import BrandLogo from '@/components/shared/BrandLogo.vue'
import { X, ArrowRight, ArrowUp, RotateCcw, MessageCircleQuestion } from 'lucide-vue-next'

interface ChatLine {
  id: number
  role: 'user' | 'assistant'
  text: string
  links: AssistantDestination[]
  pending?: boolean
}

const STORAGE_KEY = 'help_assistant_messages'
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const { isOpen, close, toggle } = useHelpAssistant()

const isChatRoute = computed(() => route.name === 'chat' || route.name === 'chat-conversation')

// Only pages this user can open (same rules as the sidebar)
const allowed = computed(() => DESTINATIONS.filter(d => {
  if (d.superAdminOnly) return !!authStore.user?.is_super_admin
  return !d.permission || authStore.hasPermission(d.permission, 'read')
}))
const byId = (ids: string[]) => ids
  .map(id => allowed.value.find(d => d.id === id))
  .filter((d): d is AssistantDestination => !!d)

function loadHistory(): ChatLine[] {
  try {
    const saved = JSON.parse(sessionStorage.getItem(STORAGE_KEY) || '[]') as ChatLine[]
    return saved.filter(m => !m.pending)
  } catch {
    return []
  }
}
const messages = ref<ChatLine[]>(loadHistory())
let nextId = messages.value.reduce((max, m) => Math.max(max, m.id), 0) + 1
watch(messages, (list) => {
  try { sessionStorage.setItem(STORAGE_KEY, JSON.stringify(list.filter(m => !m.pending).slice(-30))) } catch { /* storage unavailable */ }
}, { deep: true })

const input = ref('')
const isThinking = ref(false)
// Remember for the session that no AI provider is configured, so we stop asking
const aiUnavailable = ref(false)
const listRef = ref<HTMLElement | null>(null)
const inputRef = ref<HTMLTextAreaElement | null>(null)

const suggestions = computed(() => suggestionsFor(route.path))

function push(line: Omit<ChatLine, 'id'>): ChatLine {
  const m = { ...line, id: nextId++ }
  messages.value.push(m)
  scrollToEnd()
  return messages.value[messages.value.length - 1]
}

function scrollToEnd() {
  nextTick(() => { if (listRef.value) listRef.value.scrollTop = listRef.value.scrollHeight })
}

function go(dest: AssistantDestination) {
  router.push(dest.path)
  // On phones the panel covers the page, so get out of the way
  if (window.innerWidth < 640) close()
}

async function send(text?: string) {
  const question = (text ?? input.value).trim()
  if (!question || isThinking.value) return
  input.value = ''
  push({ role: 'user', text: question, links: [] })

  const match = matchLocally(question, allowed.value)

  if (match.navigateTo) {
    push({ role: 'assistant', text: t('assistant.opening', { page: match.navigateTo.title }), links: [match.navigateTo] })
    go(match.navigateTo)
    return
  }
  if (match.restricted) {
    push({ role: 'assistant', text: t('assistant.noAccess', { page: match.restricted.title }), links: [] })
    return
  }
  if (match.faq) {
    push({ role: 'assistant', text: faqText(match.faq), links: byId(match.faq.links) })
    return
  }

  if (!aiUnavailable.value) {
    isThinking.value = true
    const pending = push({ role: 'assistant', text: '', links: [], pending: true })
    try {
      const history = messages.value
        .filter(m => !m.pending && m.text)
        .slice(-8)
        .map(m => ({ role: m.role, content: m.text }))
      const res = await assistantService.chat({
        messages: history,
        current_page: `${document.title} (${route.path})`,
        destinations: allowed.value.map(d => ({ id: d.id, title: d.title, description: d.description })),
      })
      Object.assign(pending, { text: res.data.data.reply, links: byId(res.data.data.links || []), pending: false })
      scrollToEnd()
      return
    } catch (e: any) {
      messages.value = messages.value.filter(m => m.id !== pending.id)
      if (e?.response?.status === 503) aiUnavailable.value = true
    } finally {
      isThinking.value = false
    }
  }

  // Offline fallback: best guess from the built-in answers, else nearby pages
  if (match.weakFaq) {
    push({ role: 'assistant', text: faqText(match.weakFaq), links: byId(match.weakFaq.links) })
  } else if (match.destinations.length) {
    push({ role: 'assistant', text: t('assistant.mightHelp'), links: match.destinations })
  } else {
    push({ role: 'assistant', text: t('assistant.noMatch'), links: [] })
  }
}

function faqText(faq: AssistantFaq) {
  return faq.ownerNote && authStore.user?.is_super_admin ? `${faq.answer}\n\n${faq.ownerNote}` : faq.answer
}

function reset() {
  messages.value = []
  input.value = ''
  nextTick(() => inputRef.value?.focus())
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    send()
  }
}

function onGlobalKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && isOpen.value) close()
}
onMounted(() => window.addEventListener('keydown', onGlobalKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onGlobalKey))

watch(isOpen, (open) => {
  if (open) {
    scrollToEnd()
    nextTick(() => inputRef.value?.focus())
  }
})

function escapeHtml(s: string) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}
/** Minimal, safe formatting: **bold**, `code`, line breaks */
function format(text: string) {
  return escapeHtml(text)
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\n/g, '<br>')
}
</script>

<template>
  <!-- Launcher: hidden on the chat screen, where it would sit on the composer;
       the sidebar's Help item opens the panel there -->
  <button
    v-if="!isChatRoute && !isOpen"
    type="button"
    class="help-launcher"
    :aria-label="$t('assistant.open')"
    @click="toggle"
  >
    <MessageCircleQuestion class="h-6 w-6" />
  </button>

  <Transition name="help-panel">
    <section
      v-if="isOpen"
      class="help-panel"
      role="dialog"
      aria-modal="false"
      :aria-label="$t('assistant.title')"
    >
      <header class="flex items-center gap-3 px-4 py-3 border-b border-white/[0.08] light:border-gray-200">
        <BrandLogo :show-text="false" mark-class="h-7 w-7" />
        <div class="flex-1 min-w-0">
          <p class="text-sm font-semibold leading-tight">{{ $t('assistant.title') }}</p>
          <p class="text-xs text-muted-foreground truncate">{{ $t('assistant.subtitle') }}</p>
        </div>
        <button v-if="messages.length" type="button" class="help-icon-btn" :title="$t('assistant.newChat')" :aria-label="$t('assistant.newChat')" @click="reset">
          <RotateCcw class="h-4 w-4" />
        </button>
        <button type="button" class="help-icon-btn" :aria-label="$t('common.close')" @click="close">
          <X class="h-4 w-4" />
        </button>
      </header>

      <div ref="listRef" class="flex-1 overflow-y-auto px-4 py-4 space-y-3" aria-live="polite">
        <!-- Welcome + suggestions -->
        <div v-if="!messages.length" class="space-y-4">
          <div class="help-bubble help-bubble-bot">
            <p class="font-medium">{{ $t('assistant.greeting', { name: authStore.user?.full_name?.split(' ')[0] || '' }) }}</p>
            <p class="mt-1 text-muted-foreground">{{ $t('assistant.intro') }}</p>
          </div>
          <div>
            <p class="mb-2 text-xs font-medium text-muted-foreground">{{ $t('assistant.tryAsking') }}</p>
            <div class="flex flex-col items-start gap-1.5">
              <button v-for="q in suggestions" :key="q" type="button" class="help-suggestion" @click="send(q)">{{ q }}</button>
            </div>
          </div>
        </div>

        <template v-for="m in messages" :key="m.id">
          <div v-if="m.role === 'user'" class="flex justify-end">
            <div class="help-bubble help-bubble-user">{{ m.text }}</div>
          </div>
          <div v-else class="space-y-2">
            <div v-if="m.pending" class="help-bubble help-bubble-bot inline-flex items-center gap-1" :aria-label="$t('assistant.thinking')">
              <span class="help-dot" /><span class="help-dot" /><span class="help-dot" />
            </div>
            <!-- eslint-disable-next-line vue/no-v-html -->
            <div v-else class="help-bubble help-bubble-bot" v-html="format(m.text)" />
            <div v-if="m.links.length" class="flex flex-wrap gap-1.5">
              <button v-for="l in m.links" :key="l.id" type="button" class="help-link" @click="go(l)">
                {{ l.title }}<ArrowRight class="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
        </template>
      </div>

      <form class="flex items-end gap-2 p-3 border-t border-white/[0.08] light:border-gray-200" @submit.prevent="send()">
        <textarea
          ref="inputRef"
          v-model="input"
          rows="1"
          maxlength="1000"
          class="help-input"
          :placeholder="$t('assistant.placeholder')"
          @keydown="onKeydown"
        />
        <button type="submit" class="help-send" :disabled="!input.trim() || isThinking" :aria-label="$t('assistant.send')">
          <ArrowUp class="h-4 w-4" />
        </button>
      </form>
    </section>
  </Transition>
</template>

<style scoped>
.help-launcher {
  position: fixed; right: 20px; bottom: 20px; z-index: 40;
  display: flex; align-items: center; justify-content: center;
  width: 48px; height: 48px; border-radius: 9999px;
  background: hsl(var(--primary)); color: hsl(var(--primary-foreground));
  box-shadow: 0 6px 20px rgba(0, 0, 0, .35);
  transition: transform .15s, background-color .15s;
}
.help-launcher:hover { transform: translateY(-1px); background: hsl(var(--primary) / .9); }
.help-panel {
  position: fixed; right: 20px; bottom: 20px; z-index: 50;
  display: flex; flex-direction: column;
  width: 380px; height: min(600px, calc(100vh - 40px));
  border-radius: 14px; overflow: hidden;
  background: hsl(var(--popover)); color: hsl(var(--popover-foreground));
  border: 1px solid rgba(255, 255, 255, .08);
  box-shadow: 0 20px 50px rgba(0, 0, 0, .45);
}
:global(.light) .help-panel { border-color: #e5e7eb; box-shadow: 0 20px 50px rgba(15, 23, 42, .18); }
@media (max-width: 639px) {
  .help-panel { right: 8px; left: 8px; bottom: 8px; width: auto; height: calc(100vh - 72px); }
}
.help-icon-btn {
  display: flex; align-items: center; justify-content: center; width: 30px; height: 30px; border-radius: 8px;
  color: hsl(var(--muted-foreground));
}
.help-icon-btn:hover { background: hsl(var(--muted)); color: hsl(var(--foreground)); }
.help-bubble { max-width: 88%; padding: 9px 12px; border-radius: 12px; font-size: 13.5px; line-height: 1.5; overflow-wrap: anywhere; }
.help-bubble-bot { background: hsl(var(--muted)); border-top-left-radius: 4px; }
.help-bubble-bot :deep(code) { font-size: 12px; padding: 1px 4px; border-radius: 4px; background: rgba(127, 127, 127, .18); }
.help-bubble-user { background: hsl(var(--primary)); color: hsl(var(--primary-foreground)); border-top-right-radius: 4px; white-space: pre-wrap; }
.help-suggestion {
  padding: 6px 12px; border-radius: 9999px; font-size: 13px; text-align: left;
  border: 1px solid hsl(var(--border)); color: hsl(var(--foreground));
  transition: background-color .15s, border-color .15s;
}
.help-suggestion:hover { background: hsl(var(--muted)); border-color: hsl(var(--primary) / .5); }
.help-link {
  display: inline-flex; align-items: center; gap: 4px; padding: 5px 10px; border-radius: 8px;
  font-size: 12.5px; font-weight: 500; color: hsl(var(--primary));
  background: hsl(var(--primary) / .1); transition: background-color .15s;
}
.help-link:hover { background: hsl(var(--primary) / .18); }
.help-input {
  flex: 1; resize: none; max-height: 120px; padding: 9px 12px; border-radius: 10px; font-size: 13.5px;
  background: hsl(var(--muted)); color: hsl(var(--foreground)); outline: none; border: 1px solid transparent;
}
.help-input:focus { border-color: hsl(var(--primary) / .6); }
.help-input::placeholder { color: hsl(var(--muted-foreground)); }
.help-send {
  display: flex; align-items: center; justify-content: center; width: 38px; height: 38px; flex-shrink: 0;
  border-radius: 9999px; background: hsl(var(--primary)); color: hsl(var(--primary-foreground));
}
.help-send:disabled { opacity: .4; }
.help-dot { width: 6px; height: 6px; border-radius: 9999px; background: hsl(var(--muted-foreground)); animation: help-blink 1.2s infinite ease-in-out; }
.help-dot:nth-child(2) { animation-delay: .15s; }
.help-dot:nth-child(3) { animation-delay: .3s; }
@keyframes help-blink { 0%, 80%, 100% { opacity: .25; } 40% { opacity: 1; } }
.help-panel-enter-active, .help-panel-leave-active { transition: opacity .15s ease, transform .15s ease; }
.help-panel-enter-from, .help-panel-leave-to { opacity: 0; transform: translateY(8px) scale(.98); }
</style>
