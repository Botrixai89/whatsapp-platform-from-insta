<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { PageHeader } from '@/components/shared'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import {
  sequencesService, templatesService, api,
  type Sequence, type SequenceInput, type SequenceStep, type SequenceEnrollment,
} from '@/services/api'
import { getErrorMessage } from '@/lib/api-utils'
import { formatDateTime } from '@/lib/utils'
import { toast } from 'vue-sonner'
import { Repeat, Plus, Pencil, Trash2, UserPlus, Users, Clock, ArrowDown, Loader2 } from 'lucide-vue-next'

interface TemplateOption { id: string; name: string; display_name?: string; body_content?: string; status: string; category: string; whatsapp_account?: string }

const { t } = useI18n()
const sequences = ref<Sequence[]>([])
const isLoading = ref(true)
const templates = ref<TemplateOption[]>([])
const accounts = ref<Array<{ name: string }>>([])

async function load() {
  isLoading.value = true
  try {
    sequences.value = (await sequencesService.list()).data.data.sequences
  } catch (e) {
    toast.error(getErrorMessage(e, t('sequences.loadFailed')))
  } finally {
    isLoading.value = false
  }
}

onMounted(async () => {
  load()
  try {
    const [tpl, acc] = await Promise.all([
      templatesService.list({ status: 'APPROVED', limit: 500 }),
      api.get('/accounts'),
    ])
    const tplData: any = (tpl.data as any).data || tpl.data
    templates.value = tplData.templates || []
    accounts.value = (acc.data as any).data?.accounts || []
  } catch {
    /* pickers stay empty; the editor explains */
  }
})

// ---- delays -------------------------------------------------------------
const UNITS = [
  { value: 'minutes', mins: 1 },
  { value: 'hours', mins: 60 },
  { value: 'days', mins: 1440 },
]
function splitDelay(mins: number) {
  if (mins > 0 && mins % 1440 === 0) return { amount: mins / 1440, unit: 'days' }
  if (mins > 0 && mins % 60 === 0) return { amount: mins / 60, unit: 'hours' }
  return { amount: mins, unit: 'minutes' }
}
function delayLabel(mins: number) {
  if (mins === 0) return t('sequences.immediately')
  const d = splitDelay(mins)
  return t(`sequences.after_${d.unit}`, { n: d.amount }, d.amount)
}

// ---- editor -------------------------------------------------------------
interface EditableStep { amount: number; unit: string; template_id: string; params: Record<string, string> }
const editorOpen = ref(false)
const editingId = ref<string | null>(null)
const isSaving = ref(false)
const form = ref({ name: '', whatsapp_account: '', is_active: true, stop_on_reply: true, trigger_tag: '' })
const steps = ref<EditableStep[]>([])

const accountTemplates = computed(() =>
  templates.value.filter(tp => !form.value.whatsapp_account || !tp.whatsapp_account || tp.whatsapp_account === form.value.whatsapp_account))

function templateVars(templateId: string): string[] {
  const body = templates.value.find(tp => tp.id === templateId)?.body_content || ''
  return [...new Set([...body.matchAll(/\{\{\s*([^}]+?)\s*\}\}/g)].map(m => m[1]))]
}

function openEditor(seq?: Sequence) {
  editingId.value = seq?.id ?? null
  form.value = seq
    ? { name: seq.name, whatsapp_account: seq.whatsapp_account, is_active: seq.is_active, stop_on_reply: seq.stop_on_reply, trigger_tag: seq.trigger_tag }
    : { name: '', whatsapp_account: accounts.value[0]?.name || '', is_active: true, stop_on_reply: true, trigger_tag: '' }
  steps.value = seq
    ? seq.steps.map(s => ({ ...splitDelay(s.delay_minutes), template_id: s.template_id, params: { ...(s.template_params as Record<string, string>) } }))
    : [{ amount: 0, unit: 'minutes', template_id: '', params: {} }]
  editorOpen.value = true
}

function addStep() {
  steps.value.push({ amount: 1, unit: 'days', template_id: '', params: {} })
}

async function save() {
  isSaving.value = true
  const payload: SequenceInput = {
    ...form.value,
    steps: steps.value.map<SequenceStep>(s => {
      const unit = UNITS.find(u => u.value === s.unit)!
      const params: Record<string, string> = {}
      for (const v of templateVars(s.template_id)) params[v] = s.params[v] ?? ''
      return { delay_minutes: Math.max(0, Math.round(Number(s.amount) || 0)) * unit.mins, template_id: s.template_id, template_params: params }
    }),
  }
  try {
    if (editingId.value) await sequencesService.update(editingId.value, payload)
    else await sequencesService.create(payload)
    editorOpen.value = false
    toast.success(t('sequences.saved'))
    load()
  } catch (e) {
    toast.error(getErrorMessage(e, t('sequences.saveFailed')))
  } finally {
    isSaving.value = false
  }
}

async function remove(seq: Sequence) {
  if (!window.confirm(t('sequences.deleteConfirm', { name: seq.name }))) return
  try {
    await sequencesService.delete(seq.id)
    sequences.value = sequences.value.filter(s => s.id !== seq.id)
  } catch (e) {
    toast.error(getErrorMessage(e, t('sequences.deleteFailed')))
  }
}

async function toggleActive(seq: Sequence) {
  try {
    await sequencesService.update(seq.id, {
      name: seq.name, whatsapp_account: seq.whatsapp_account, is_active: !seq.is_active,
      stop_on_reply: seq.stop_on_reply, trigger_tag: seq.trigger_tag, steps: seq.steps,
    })
    seq.is_active = !seq.is_active
  } catch (e) {
    toast.error(getErrorMessage(e, t('sequences.saveFailed')))
  }
}

// ---- enrol ----------------------------------------------------------------
const enrollFor = ref<Sequence | null>(null)
const enrollPhones = ref('')
const enrollTag = ref('')
const isEnrolling = ref(false)
async function enroll() {
  if (!enrollFor.value) return
  isEnrolling.value = true
  try {
    const phones = enrollPhones.value.split(/[\s,;]+/).map(p => p.replace(/[^\d]/g, '')).filter(p => p.length >= 8)
    const res = await sequencesService.enroll(enrollFor.value.id, { phones, tag: enrollTag.value.trim() || undefined })
    toast.success(t('sequences.enrolled', { n: res.data.data.enrolled, skipped: res.data.data.skipped }))
    enrollFor.value = null
    load()
  } catch (e) {
    toast.error(getErrorMessage(e, t('sequences.enrollFailed')))
  } finally {
    isEnrolling.value = false
  }
}

// ---- people in a sequence ------------------------------------------------
const peopleFor = ref<Sequence | null>(null)
const people = ref<SequenceEnrollment[]>([])
async function openPeople(seq: Sequence) {
  peopleFor.value = seq
  people.value = []
  try {
    people.value = (await sequencesService.enrollments(seq.id)).data.data.enrollments
  } catch (e) {
    toast.error(getErrorMessage(e, t('sequences.loadFailed')))
  }
}
async function stopPerson(e: SequenceEnrollment) {
  if (!peopleFor.value) return
  try {
    await sequencesService.stopEnrollment(peopleFor.value.id, e.id)
    e.status = 'stopped'
    e.stopped_reason = t('sequences.stoppedByAgent')
  } catch (err) {
    toast.error(getErrorMessage(err, t('sequences.saveFailed')))
  }
}
const statusVariant = (s: string) => ({ active: 'success', completed: 'secondary', stopped: 'outline' } as Record<string, any>)[s] || 'secondary'
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('sequences.title')" :description="$t('sequences.subtitle')" :icon="Repeat">
      <template #actions>
        <Button size="sm" @click="openEditor()"><Plus class="h-4 w-4" />{{ $t('sequences.create') }}</Button>
      </template>
    </PageHeader>

    <ScrollArea class="flex-1">
      <div class="p-6 space-y-4">
        <div v-if="isLoading" class="py-12 text-center text-sm text-muted-foreground">{{ $t('common.loading') }}…</div>
        <Card v-else-if="!sequences.length">
          <CardContent class="py-14 text-center">
            <Repeat class="mx-auto mb-3 h-6 w-6 text-muted-foreground" />
            <p class="font-medium">{{ $t('sequences.emptyTitle') }}</p>
            <p class="mx-auto mt-1 max-w-md text-sm text-muted-foreground">{{ $t('sequences.emptyDesc') }}</p>
            <Button class="mt-4" size="sm" @click="openEditor()"><Plus class="h-4 w-4" />{{ $t('sequences.create') }}</Button>
          </CardContent>
        </Card>

        <Card v-for="seq in sequences" :key="seq.id">
          <CardContent class="p-5">
            <div class="flex flex-wrap items-start gap-4">
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2">
                  <p class="font-semibold">{{ seq.name }}</p>
                  <Badge :variant="seq.is_active ? 'success' : 'secondary'">{{ seq.is_active ? $t('sequences.running') : $t('sequences.paused') }}</Badge>
                </div>
                <p class="mt-1 text-xs text-muted-foreground">
                  {{ $t('sequences.stepsCount', { n: seq.steps.length }, seq.steps.length) }} · {{ seq.whatsapp_account }}
                  <template v-if="seq.trigger_tag"> · {{ $t('sequences.autoTag', { tag: seq.trigger_tag }) }}</template>
                  <template v-if="seq.stop_on_reply"> · {{ $t('sequences.stopsOnReply') }}</template>
                </p>
              </div>
              <div class="flex gap-5 text-center text-xs">
                <div><p class="text-lg font-semibold tabular-nums">{{ seq.active }}</p><p class="text-muted-foreground">{{ $t('sequences.inProgress') }}</p></div>
                <div><p class="text-lg font-semibold tabular-nums">{{ seq.completed }}</p><p class="text-muted-foreground">{{ $t('sequences.finished') }}</p></div>
                <div><p class="text-lg font-semibold tabular-nums">{{ seq.stopped }}</p><p class="text-muted-foreground">{{ $t('sequences.stopped') }}</p></div>
              </div>
              <div class="flex items-center gap-1">
                <Switch :checked="seq.is_active" :aria-label="$t('sequences.running')" @update:checked="toggleActive(seq)" />
                <Button variant="outline" size="sm" class="ml-2" @click="enrollFor = seq; enrollPhones = ''; enrollTag = ''"><UserPlus class="h-4 w-4" />{{ $t('sequences.addPeople') }}</Button>
                <Button variant="ghost" size="icon-sm" :aria-label="$t('sequences.people')" @click="openPeople(seq)"><Users class="h-4 w-4" /></Button>
                <Button variant="ghost" size="icon-sm" :aria-label="$t('common.edit')" @click="openEditor(seq)"><Pencil class="h-4 w-4" /></Button>
                <Button variant="ghost" size="icon-sm" :aria-label="$t('common.delete')" @click="remove(seq)"><Trash2 class="h-4 w-4 text-destructive" /></Button>
              </div>
            </div>
            <!-- step timeline -->
            <ol class="mt-4 flex flex-wrap items-center gap-2 text-xs">
              <li v-for="(step, i) in seq.steps" :key="step.id || i" class="flex items-center gap-2">
                <span class="rounded-full border px-2.5 py-1">
                  <Clock class="mr-1 inline h-3 w-3 text-muted-foreground" />{{ delayLabel(step.delay_minutes) }} →
                  <span class="font-medium">{{ templates.find(tp => tp.id === step.template_id)?.name || $t('sequences.templateMissing') }}</span>
                </span>
                <span v-if="i < seq.steps.length - 1" class="text-muted-foreground">›</span>
              </li>
            </ol>
          </CardContent>
        </Card>
      </div>
    </ScrollArea>

    <!-- Create / edit -->
    <Dialog v-model:open="editorOpen">
      <DialogContent class="sm:max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{{ editingId ? $t('sequences.edit') : $t('sequences.create') }}</DialogTitle>
          <DialogDescription>{{ $t('sequences.editorDesc') }}</DialogDescription>
        </DialogHeader>
        <div class="space-y-4">
          <div class="grid gap-4 md:grid-cols-2">
            <div class="space-y-2">
              <Label for="seq-name">{{ $t('common.name') }}</Label>
              <Input id="seq-name" v-model="form.name" :placeholder="$t('sequences.namePlaceholder')" />
            </div>
            <div class="space-y-2">
              <Label>{{ $t('sequences.number') }}</Label>
              <Select v-model="form.whatsapp_account">
                <SelectTrigger><SelectValue :placeholder="$t('sequences.chooseNumber')" /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="a in accounts" :key="a.name" :value="a.name">{{ a.name }}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <div class="grid gap-4 md:grid-cols-2">
            <div class="space-y-2">
              <Label for="seq-tag">{{ $t('sequences.triggerTag') }}</Label>
              <Input id="seq-tag" v-model="form.trigger_tag" :placeholder="$t('sequences.triggerTagPlaceholder')" />
              <p class="text-xs text-muted-foreground">{{ $t('sequences.triggerTagHint') }}</p>
            </div>
            <div class="space-y-3 pt-1">
              <label class="flex items-center justify-between gap-3 text-sm">{{ $t('sequences.stopOnReply') }}<Switch v-model:checked="form.stop_on_reply" /></label>
              <label class="flex items-center justify-between gap-3 text-sm">{{ $t('sequences.active') }}<Switch v-model:checked="form.is_active" /></label>
            </div>
          </div>

          <div class="space-y-3">
            <Label>{{ $t('sequences.steps') }}</Label>
            <p v-if="!templates.length" class="rounded-lg border border-dashed p-3 text-xs text-muted-foreground">{{ $t('sequences.noTemplates') }}</p>
            <template v-for="(step, i) in steps" :key="i">
              <div class="space-y-3 rounded-lg border p-3">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-xs font-semibold">{{ $t('sequences.stepN', { n: i + 1 }) }}</span>
                  <span class="text-xs text-muted-foreground">{{ i === 0 ? $t('sequences.sendAfterEnrol') : $t('sequences.sendAfterPrevious') }}</span>
                  <Input v-model.number="step.amount" type="number" min="0" class="h-8 w-20" />
                  <Select v-model="step.unit">
                    <SelectTrigger class="h-8 w-28"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem v-for="u in UNITS" :key="u.value" :value="u.value">{{ $t(`sequences.unit_${u.value}`) }}</SelectItem>
                    </SelectContent>
                  </Select>
                  <Button v-if="steps.length > 1" variant="ghost" size="icon-sm" class="ml-auto" :aria-label="$t('common.remove')" @click="steps.splice(i, 1)">
                    <Trash2 class="h-4 w-4 text-destructive" />
                  </Button>
                </div>
                <Select v-model="step.template_id">
                  <SelectTrigger><SelectValue :placeholder="$t('sequences.chooseTemplate')" /></SelectTrigger>
                  <SelectContent>
                    <SelectItem v-for="tp in accountTemplates" :key="tp.id" :value="tp.id">{{ tp.display_name || tp.name }} · {{ tp.category }}</SelectItem>
                  </SelectContent>
                </Select>
                <div v-if="templateVars(step.template_id).length" class="grid gap-2 md:grid-cols-2">
                  <div v-for="v in templateVars(step.template_id)" :key="v" class="space-y-1">
                    <Label class="text-xs">{{ `\{\{${v}\}\}` }}</Label>
                    <Input v-model="step.params[v]" placeholder="{name}" />
                  </div>
                  <p class="text-xs text-muted-foreground md:col-span-2">{{ $t('sequences.paramsHint') }}</p>
                </div>
              </div>
              <ArrowDown v-if="i < steps.length - 1" class="mx-auto h-4 w-4 text-muted-foreground" />
            </template>
            <Button variant="outline" size="sm" :disabled="steps.length >= 20" @click="addStep"><Plus class="h-4 w-4" />{{ $t('sequences.addStep') }}</Button>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="editorOpen = false">{{ $t('common.cancel') }}</Button>
          <Button :disabled="isSaving" @click="save"><Loader2 v-if="isSaving" class="h-4 w-4 animate-spin" />{{ $t('common.save') }}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Enrol people -->
    <Dialog :open="!!enrollFor" @update:open="(o: boolean) => { if (!o) enrollFor = null }">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ $t('sequences.addPeopleTitle', { name: enrollFor?.name }) }}</DialogTitle>
          <DialogDescription>{{ $t('sequences.addPeopleDesc') }}</DialogDescription>
        </DialogHeader>
        <div class="space-y-4">
          <div class="space-y-2">
            <Label for="enrol-phones">{{ $t('sequences.phones') }}</Label>
            <Textarea id="enrol-phones" v-model="enrollPhones" :rows="4" placeholder="919876543210&#10;919812345678" />
          </div>
          <div class="space-y-2">
            <Label for="enrol-tag">{{ $t('sequences.orTag') }}</Label>
            <Input id="enrol-tag" v-model="enrollTag" :placeholder="$t('sequences.tagPlaceholder')" />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="enrollFor = null">{{ $t('common.cancel') }}</Button>
          <Button :disabled="isEnrolling || (!enrollPhones.trim() && !enrollTag.trim())" @click="enroll">
            <Loader2 v-if="isEnrolling" class="h-4 w-4 animate-spin" />{{ $t('sequences.addPeople') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- People in a sequence -->
    <Sheet :open="!!peopleFor" @update:open="(o: boolean) => { if (!o) peopleFor = null }">
      <SheetContent class="w-full sm:max-w-lg overflow-y-auto">
        <SheetHeader>
          <SheetTitle>{{ peopleFor?.name }}</SheetTitle>
          <SheetDescription>{{ $t('sequences.peopleDesc') }}</SheetDescription>
        </SheetHeader>
        <p v-if="!people.length" class="py-8 text-center text-sm text-muted-foreground">{{ $t('sequences.noPeople') }}</p>
        <ul v-else class="mt-4 divide-y divide-white/[0.06] light:divide-gray-100">
          <li v-for="p in people" :key="p.id" class="flex items-start gap-3 py-3 text-sm">
            <div class="min-w-0 flex-1">
              <p class="font-medium">{{ p.contact?.profile_name || p.contact?.phone_number }}</p>
              <p class="text-xs text-muted-foreground">
                {{ $t('sequences.stepsSent', { n: p.steps_sent, total: peopleFor?.steps.length ?? 0 }) }}
                <template v-if="p.status === 'active' && p.next_run_at"> · {{ $t('sequences.nextAt', { at: formatDateTime(p.next_run_at) }) }}</template>
                <template v-if="p.stopped_reason"> · {{ p.stopped_reason }}</template>
              </p>
              <p v-if="p.last_error" class="text-xs text-destructive">{{ p.last_error }}</p>
            </div>
            <Badge :variant="statusVariant(p.status)">{{ $t(`sequences.status_${p.status}`) }}</Badge>
            <Button v-if="p.status === 'active'" variant="ghost" size="sm" @click="stopPerson(p)">{{ $t('sequences.stop') }}</Button>
          </li>
        </ul>
      </SheetContent>
    </Sheet>
  </div>
</template>
