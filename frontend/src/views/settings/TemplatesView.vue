<script setup lang="ts">
import { ref, onMounted, watch, computed } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { PageHeader, SearchInput, DataTable, IconButton, DeleteConfirmDialog, ErrorState, type Column } from '@/components/shared'
import { api, templatesService, templateAIService } from '@/services/api'
import TemplateStatusCards from './templates/TemplateStatusCards.vue'
import TemplateGallery from './templates/TemplateGallery.vue'
import TemplateApiDialog from './templates/TemplateApiDialog.vue'
import WhatsAppTemplatePreview from './templates/WhatsAppTemplatePreview.vue'
import { useAuthStore } from '@/stores/auth'
import { useOrganizationsStore } from '@/stores/organizations'
import { toast } from 'vue-sonner'
import { Plus, RefreshCw, FileText, Pencil, Trash2, Loader2, MessageSquare, Image, FileIcon, Video, Sparkles, BarChart3, CopyPlus, Eye, Code2, LayoutTemplate } from 'lucide-vue-next'
import { getErrorMessage } from '@/lib/api-utils'
import { useSearchPagination } from '@/composables/useSearchPagination'
import { getQualityBadgeClass, getQualityRatingLabel, formatDate } from '@/lib/utils'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const canWrite = computed(() => authStore.hasPermission('templates', 'write'))

// Gallery of ready-made templates vs. the business's own templates
const tab = ref(route.query.tab === 'gallery' ? 'gallery' : 'saved')
watch(tab, (v) => router.replace({ query: { ...route.query, tab: v === 'gallery' ? 'gallery' : undefined } }))

interface WhatsAppAccount {
  id: string
  name: string
  phone_id: string
}

interface Template {
  id: string
  whatsapp_account: string
  meta_template_id: string
  name: string
  display_name: string
  language: string
  category: string
  status: string
  header_type: string
  header_content: string
  body_content: string
  footer_content: string
  buttons: any[]
  sample_values: any[]
  created_at: string
  updated_at: string
  quality_rating?: string
  add_security_recommendation?: boolean
}

const organizationsStore = useOrganizationsStore()

const templates = ref<Template[]>([])
const accounts = ref<WhatsAppAccount[]>([])
const isLoading = ref(true)
const error = ref<string | null>(null)
const isSyncing = ref(false)
const selectedAccount = ref<string>(localStorage.getItem('templates_selected_account') || 'all')

// Delete dialog state
const deleteDialogOpen = ref(false)
const templateToDelete = ref<Template | null>(null)
const isDeleting = ref(false)

const { searchQuery, currentPage, totalItems, perPage, setPageSize, handlePageChange, resetAndFetch } = useSearchPagination({
  fetchFn: () => fetchTemplates(),
})

// Filters
const filterStatus = ref('all')
const filterCategory = ref('all')
const filterUpdated = ref('any')
const statusFilterOptions = ['APPROVED', 'PENDING', 'REJECTED', 'PAUSED', 'DISABLED', 'DRAFT']
const categoryFilterOptions = ['MARKETING', 'UTILITY', 'AUTHENTICATION']
const updatedFilterOptions = [
  { value: 'any', days: 0 },
  { value: '7', days: 7 },
  { value: '30', days: 30 },
  { value: '90', days: 90 },
]
const hasFilters = computed(() => filterStatus.value !== 'all' || filterCategory.value !== 'all' || filterUpdated.value !== 'any' || !!searchQuery.value)
function clearFilters() {
  filterStatus.value = 'all'
  filterCategory.value = 'all'
  filterUpdated.value = 'any'
  searchQuery.value = ''
}
watch([filterStatus, filterCategory, filterUpdated], () => resetAndFetch())

function updatedFrom(): string | undefined {
  const days = updatedFilterOptions.find(o => o.value === filterUpdated.value)?.days
  if (!days) return undefined
  const d = new Date()
  d.setDate(d.getDate() - days)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

// Preview and API dialogs
const previewTemplate = ref<Template | null>(null)
const previewOpen = computed({ get: () => !!previewTemplate.value, set: (v) => { if (!v) previewTemplate.value = null } })
const apiTemplate = ref<Template | null>(null)
const apiOpen = ref(false)
function openApi(tpl: Template) {
  apiTemplate.value = tpl
  apiOpen.value = true
}
function duplicate(tpl: Template) {
  router.push({ path: '/templates/new', query: { copy: tpl.id } })
}

const columns = computed<Column<Template>[]>(() => [
  { key: 'name', label: t('templates.name'), sortable: true },
  { key: 'category', label: t('templates.category'), sortable: true },
  { key: 'status', label: t('templates.status'), sortable: true },
  { key: 'quality_rating', label: t('templates.qualityRating'), sortable: true },
  { key: 'language', label: t('templates.language'), sortable: true },
  { key: 'updated_at', label: t('templates.lastUpdated'), sortable: true },
  { key: 'actions', label: t('common.actions'), align: 'right' },
])

const sortKey = ref('updated_at')
const sortDirection = ref<'asc' | 'desc'>('desc')

const languages = [
  { code: 'af', name: 'Afrikaans' },
  { code: 'sq', name: 'Albanian' },
  { code: 'ar', name: 'Arabic' },
  { code: 'az', name: 'Azerbaijani' },
  { code: 'bn', name: 'Bengali' },
  { code: 'bg', name: 'Bulgarian' },
  { code: 'ca', name: 'Catalan' },
  { code: 'zh_CN', name: 'Chinese (CHN)' },
  { code: 'zh_HK', name: 'Chinese (HKG)' },
  { code: 'zh_TW', name: 'Chinese (TAI)' },
  { code: 'hr', name: 'Croatian' },
  { code: 'cs', name: 'Czech' },
  { code: 'da', name: 'Danish' },
  { code: 'nl', name: 'Dutch' },
  { code: 'en', name: 'English' },
  { code: 'en_GB', name: 'English (UK)' },
  { code: 'en_US', name: 'English (US)' },
  { code: 'et', name: 'Estonian' },
  { code: 'fil', name: 'Filipino' },
  { code: 'fi', name: 'Finnish' },
  { code: 'fr', name: 'French' },
  { code: 'ka', name: 'Georgian' },
  { code: 'de', name: 'German' },
  { code: 'el', name: 'Greek' },
  { code: 'gu', name: 'Gujarati' },
  { code: 'ha', name: 'Hausa' },
  { code: 'he', name: 'Hebrew' },
  { code: 'hi', name: 'Hindi' },
  { code: 'hu', name: 'Hungarian' },
  { code: 'id', name: 'Indonesian' },
  { code: 'ga', name: 'Irish' },
  { code: 'it', name: 'Italian' },
  { code: 'ja', name: 'Japanese' },
  { code: 'kn', name: 'Kannada' },
  { code: 'kk', name: 'Kazakh' },
  { code: 'rw_RW', name: 'Kinyarwanda' },
  { code: 'ko', name: 'Korean' },
  { code: 'ky_KG', name: 'Kyrgyz (Kyrgyzstan)' },
  { code: 'lo', name: 'Lao' },
  { code: 'lv', name: 'Latvian' },
  { code: 'lt', name: 'Lithuanian' },
  { code: 'mk', name: 'Macedonian' },
  { code: 'ms', name: 'Malay' },
  { code: 'ml', name: 'Malayalam' },
  { code: 'mr', name: 'Marathi' },
  { code: 'nb', name: 'Norwegian (Bokm\u00e5l)' },
  { code: 'fa', name: 'Persian' },
  { code: 'pl', name: 'Polish' },
  { code: 'pt_BR', name: 'Portuguese (BR)' },
  { code: 'pt_PT', name: 'Portuguese (POR)' },
  { code: 'pa', name: 'Punjabi' },
  { code: 'ro', name: 'Romanian' },
  { code: 'ru', name: 'Russian' },
  { code: 'sr', name: 'Serbian' },
  { code: 'sk', name: 'Slovak' },
  { code: 'sl', name: 'Slovenian' },
  { code: 'es', name: 'Spanish' },
  { code: 'es_AR', name: 'Spanish (ARG)' },
  { code: 'es_MX', name: 'Spanish (MEX)' },
  { code: 'es_ES', name: 'Spanish (SPA)' },
  { code: 'sw', name: 'Swahili' },
  { code: 'sv', name: 'Swedish' },
  { code: 'ta', name: 'Tamil' },
  { code: 'te', name: 'Telugu' },
  { code: 'th', name: 'Thai' },
  { code: 'tr', name: 'Turkish' },
  { code: 'uk', name: 'Ukrainian' },
  { code: 'ur', name: 'Urdu' },
  { code: 'uz', name: 'Uzbek' },
  { code: 'vi', name: 'Vietnamese' },
  { code: 'zu', name: 'Zulu' },
]

function getLanguageName(code: string): string {
  return languages.find(l => l.code === code)?.name || code
}

// Refetch data when organization changes
watch(() => organizationsStore.selectedOrgId, async () => {
  await fetchAccounts()
  await fetchTemplates()
})

// Template counts by status
const statusCounts = ref<Record<string, number> | null>(null)
async function fetchStatusCounts() {
  try {
    const res = await templateAIService.analytics({ account: selectedAccount.value !== 'all' ? selectedAccount.value : undefined })
    statusCounts.value = res.data.data.status_counts
  } catch {
    statusCounts.value = null
  }
}
watch(templates, fetchStatusCounts)

onMounted(async () => {
  await fetchAccounts()
  await fetchTemplates()
})

async function fetchAccounts() {
  try {
    const response = await api.get('/accounts')
    accounts.value = response.data.data?.accounts || []
    // Validate stored account still exists, fallback to 'all' if not
    if (selectedAccount.value !== 'all' && !accounts.value.some(a => a.name === selectedAccount.value)) {
      selectedAccount.value = 'all'
      localStorage.setItem('templates_selected_account', 'all')
    }
  } catch (error) {
    console.error('Failed to fetch accounts:', error)
  }
}

function onAccountChange(value: string | number | bigint | Record<string, any> | null) {
  if (typeof value !== 'string') return
  localStorage.setItem('templates_selected_account', value)
  currentPage.value = 1
  fetchTemplates()
}

async function fetchTemplates() {
  isLoading.value = true
  error.value = null
  try {
    const response = await templatesService.list({
      account: selectedAccount.value !== 'all' ? selectedAccount.value : undefined,
      search: searchQuery.value || undefined,
      status: filterStatus.value !== 'all' ? filterStatus.value : undefined,
      category: filterCategory.value !== 'all' ? filterCategory.value : undefined,
      from: updatedFrom(),
      page: currentPage.value,
      limit: perPage.value
    })
    const data = (response.data as any).data || response.data
    templates.value = data.templates || []
    totalItems.value = data.total ?? templates.value.length
  } catch (err: any) {
    console.error('Failed to fetch templates:', err)
    error.value = t('templates.errorLoadingTemplates')
    toast.error(t('common.failedLoad', { resource: t('resources.templates') }))
    templates.value = []
  } finally {
    isLoading.value = false
  }
}

// Syncs the selected account, or every account when "All accounts" is selected
async function syncTemplates() {
  const names = selectedAccount.value && selectedAccount.value !== 'all'
    ? [selectedAccount.value]
    : accounts.value.map(a => a.name)
  if (!names.length) {
    toast.error(t('templates.selectAccountFirst'))
    return
  }

  isSyncing.value = true
  try {
    let message = ''
    for (const name of names) {
      const response = await api.post('/templates/sync', { whatsapp_account: name })
      message = response.data.data.message
    }
    toast.success(names.length === 1 && message ? message : t('templates.syncSuccess'))
    await fetchTemplates()
  } catch (error) {
    toast.error(getErrorMessage(error, t('templates.syncFailed')))
  } finally {
    isSyncing.value = false
  }
}

function openDeleteDialog(template: Template) {
  templateToDelete.value = template
  deleteDialogOpen.value = true
}

async function confirmDeleteTemplate() {
  if (!templateToDelete.value) return

  isDeleting.value = true
  try {
    await api.delete(`/templates/${templateToDelete.value.id}`)
    toast.success(t('common.deletedSuccess', { resource: t('resources.Template') }))
    deleteDialogOpen.value = false
    templateToDelete.value = null
    await fetchTemplates()
  } catch (error) {
    toast.error(getErrorMessage(error, t('common.failedDelete', { resource: t('resources.template') })))
  } finally {
    isDeleting.value = false
  }
}

// Dark-first: default is dark mode, light: prefix for light mode
function getStatusBadgeClass(status: string) {
  switch (status) {
    case 'APPROVED':
      return 'bg-green-900 text-green-300 light:bg-green-100 light:text-green-800'
    case 'PENDING':
      return 'bg-yellow-900 text-yellow-300 light:bg-yellow-100 light:text-yellow-800'
    case 'REJECTED':
      return 'bg-red-900 text-red-300 light:bg-red-100 light:text-red-800'
    case 'DRAFT':
      return 'bg-gray-800 text-gray-300 light:bg-gray-100 light:text-gray-800'
    default:
      return 'bg-gray-800 text-gray-300 light:bg-gray-100 light:text-gray-800'
  }
}

function getCategoryBadgeClass(category: string) {
  switch (category) {
    case 'UTILITY':
      return 'bg-blue-900 text-blue-300 light:bg-blue-100 light:text-blue-800'
    case 'MARKETING':
      return 'bg-purple-900 text-purple-300 light:bg-purple-100 light:text-purple-800'
    case 'AUTHENTICATION':
      return 'bg-orange-900 text-orange-300 light:bg-orange-100 light:text-orange-800'
    default:
      return 'bg-gray-800 text-gray-300 light:bg-gray-100 light:text-gray-800'
  }
}

function getHeaderIcon(type: string) {
  switch (type) {
    case 'IMAGE':
      return Image
    case 'VIDEO':
      return Video
    case 'DOCUMENT':
      return FileIcon
    default:
      return MessageSquare
  }
}
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('templates.title')" :subtitle="$t('templates.subtitle')" :icon="FileText">
      <template #actions>
        <Button variant="outline" size="sm" @click="syncTemplates" :disabled="isSyncing || !accounts.length">
          <Loader2 v-if="isSyncing" class="h-4 w-4 mr-2 animate-spin" />
          <RefreshCw v-else class="h-4 w-4 mr-2" />
          {{ $t('templates.syncFromMeta') }}
        </Button>
        <RouterLink to="/templates/analytics">
          <Button variant="outline" size="sm">
            <BarChart3 class="h-4 w-4 mr-2" />
            {{ $t('templateAI.analyticsButton') }}
          </Button>
        </RouterLink>
        <RouterLink to="/templates/new">
          <Button variant="outline" size="sm">
            <Plus class="h-4 w-4 mr-2" />
            {{ $t('templates.createTemplate') }}
          </Button>
        </RouterLink>
        <RouterLink to="/templates/new?ai=1">
          <Button size="sm">
            <Sparkles class="h-4 w-4 mr-2" />
            {{ $t('templateAI.createWithAI') }}
          </Button>
        </RouterLink>
      </template>
    </PageHeader>

    <ScrollArea class="flex-1">
      <div class="p-6">
        <Tabs v-model="tab" class="space-y-6">
          <TabsList class="h-auto bg-white/[0.04] p-1 ring-1 ring-white/[0.08] light:bg-gray-100 light:ring-gray-200">
            <TabsTrigger value="saved" class="px-4 py-1.5">
              <FileText class="mr-2 h-4 w-4" />{{ $t('templates.savedTemplates') }}
            </TabsTrigger>
            <TabsTrigger value="gallery" class="px-4 py-1.5">
              <LayoutTemplate class="mr-2 h-4 w-4" />{{ $t('templateGallery.title') }}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="gallery" class="mt-0">
            <TemplateGallery :can-write="canWrite" />
          </TabsContent>

          <TabsContent value="saved" class="mt-0 space-y-6">
          <TemplateStatusCards :counts="statusCounts" />
          <ErrorState
            v-if="error && !isLoading"
            :title="$t('common.loadErrorTitle')"
            :description="error"
            :retry-label="$t('common.retry')"
            @retry="fetchTemplates"
          />
          <Card v-else>
            <CardHeader class="space-y-4">
              <div>
                <CardTitle>{{ $t('templates.yourTemplates') }}</CardTitle>
                <CardDescription>{{ $t('templates.yourTemplatesDesc') }}</CardDescription>
              </div>
              <div class="flex flex-wrap items-center gap-2">
                <Select v-model="selectedAccount" @update:model-value="onAccountChange">
                  <SelectTrigger class="w-[170px]" :aria-label="$t('templates.account')">
                    <SelectValue :placeholder="$t('templates.allAccounts')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{{ $t('templates.allAccounts') }}</SelectItem>
                    <SelectItem v-for="account in accounts" :key="account.id" :value="account.name">
                      {{ account.name }}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <Select v-model="filterStatus">
                  <SelectTrigger class="w-[150px]" :aria-label="$t('templates.status')">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{{ $t('templates.allStatuses') }}</SelectItem>
                    <SelectItem v-for="st in statusFilterOptions" :key="st" :value="st">{{ $t(`templates.statusNames.${st}`) }}</SelectItem>
                  </SelectContent>
                </Select>
                <Select v-model="filterCategory">
                  <SelectTrigger class="w-[160px]" :aria-label="$t('templates.category')">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{{ $t('templates.allCategories') }}</SelectItem>
                    <SelectItem v-for="c in categoryFilterOptions" :key="c" :value="c">{{ $t(`templates.categoryNames.${c}`) }}</SelectItem>
                  </SelectContent>
                </Select>
                <Select v-model="filterUpdated">
                  <SelectTrigger class="w-[160px]" :aria-label="$t('templates.lastUpdated')">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem v-for="o in updatedFilterOptions" :key="o.value" :value="o.value">
                      {{ o.days ? $t('templates.updatedWithin', { n: o.days }) : $t('templates.updatedAnyTime') }}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <SearchInput v-model="searchQuery" :placeholder="$t('templates.searchTemplates') + '...'" class="w-full sm:w-56" />
                <Button v-if="hasFilters" variant="ghost" size="sm" @click="clearFilters">{{ $t('common.clearFilters') }}</Button>
              </div>
            </CardHeader>
            <CardContent>
              <DataTable
                :items="templates"
                :columns="columns"
                :is-loading="isLoading"
                :empty-icon="FileText"
                :empty-title="hasFilters ? $t('templates.noMatchingTemplates') : $t('templates.noTemplatesFound')"
                :empty-description="hasFilters ? $t('templates.noMatchingTemplatesDesc') : $t('templates.noTemplatesFoundDesc')"
                server-pagination
                :current-page="currentPage"
                :total-items="totalItems"
                :page-size="perPage"
                :page-size-options="[10, 25, 50, 100]"
                :item-name="$t('resources.templates')"
                @page-change="handlePageChange"
                @update:page-size="setPageSize"
                v-model:sort-key="sortKey"
                v-model:sort-direction="sortDirection"
              >
                <template #cell-name="{ item: template }">
                  <RouterLink :to="`/templates/${template.id}`" class="flex items-center gap-2.5 text-inherit no-underline hover:opacity-80">
                    <component :is="getHeaderIcon(template.header_type)" class="h-4 w-4 shrink-0 text-muted-foreground" :aria-label="template.header_type || 'TEXT'" />
                    <span class="min-w-0">
                      <span class="block truncate font-medium">{{ template.display_name || template.name }}</span>
                      <span v-if="template.display_name && template.display_name !== template.name" class="block truncate text-xs font-mono text-muted-foreground">{{ template.name }}</span>
                    </span>
                  </RouterLink>
                </template>
                <template #cell-category="{ item: template }">
                  <Badge :class="getCategoryBadgeClass(template.category)" class="text-xs">
                    {{ template.category }}
                  </Badge>
                </template>
                <template #cell-status="{ item: template }">
                  <Badge :class="getStatusBadgeClass(template.status)" class="text-xs">
                    {{ template.status }}
                  </Badge>
                </template>
                <template #cell-quality_rating="{ item: template }">
                  <Badge v-if="template.quality_rating" :class="getQualityBadgeClass(template.quality_rating)" class="text-xs">
                    {{ getQualityRatingLabel(template.quality_rating, t) }}
                  </Badge>
                  <span v-else class="text-muted-foreground text-xs">—</span>
                </template>
                <template #cell-language="{ item: template }">
                  <span class="text-muted-foreground">{{ getLanguageName(template.language) }}</span>
                </template>
                <template #cell-updated_at="{ item: template }">
                  <span class="whitespace-nowrap text-sm text-muted-foreground">{{ template.updated_at && !template.updated_at.startsWith('0001') ? formatDate(template.updated_at) : '—' }}</span>
                </template>
                <template #cell-actions="{ item: template }">
                  <div class="flex items-center justify-end gap-0.5">
                    <IconButton :icon="Eye" :label="$t('templates.preview')" class="h-8 w-8" @click="previewTemplate = template" />
                    <IconButton v-if="canWrite" :icon="CopyPlus" :label="$t('templates.duplicate')" class="h-8 w-8" @click="duplicate(template)" />
                    <IconButton v-if="template.status === 'APPROVED'" :icon="Code2" :label="$t('templates.apiCode')" class="h-8 w-8" @click="openApi(template)" />
                    <RouterLink :to="`/templates/${template.id}`">
                      <IconButton :icon="Pencil" :label="$t('common.edit')" class="h-8 w-8" />
                    </RouterLink>
                    <IconButton
                      :icon="Trash2"
                      :label="$t('common.delete')"
                      class="h-8 w-8 text-destructive"
                      @click="openDeleteDialog(template)"
                    />
                  </div>
                </template>
                <template #empty-action>
                  <div v-if="hasFilters" class="flex justify-center">
                    <Button variant="outline" size="sm" @click="clearFilters">{{ $t('common.clearFilters') }}</Button>
                  </div>
                  <div v-else class="flex flex-wrap items-center justify-center gap-2">
                    <Button variant="outline" size="sm" @click="tab = 'gallery'">
                      <LayoutTemplate class="h-4 w-4 mr-2" />
                      {{ $t('templateGallery.browse') }}
                    </Button>
                    <Button variant="outline" size="sm" @click="syncTemplates" :disabled="isSyncing || !accounts.length">
                      <RefreshCw class="h-4 w-4 mr-2" />
                      {{ $t('templates.syncFromMeta') }}
                    </Button>
                    <RouterLink to="/templates/new">
                      <Button variant="outline" size="sm">
                        <Plus class="h-4 w-4 mr-2" />
                        {{ $t('templates.createTemplate') }}
                      </Button>
                    </RouterLink>
                  </div>
                </template>
              </DataTable>
            </CardContent>
          </Card>
          </TabsContent>
        </Tabs>
      </div>
    </ScrollArea>

    <!-- Preview -->
    <Dialog v-model:open="previewOpen">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle class="flex items-center gap-2">
            {{ previewTemplate?.display_name || previewTemplate?.name }}
            <Badge v-if="previewTemplate" :class="getStatusBadgeClass(previewTemplate.status)" class="text-[10px]">{{ previewTemplate.status }}</Badge>
          </DialogTitle>
        </DialogHeader>
        <WhatsAppTemplatePreview v-if="previewTemplate" :draft="previewTemplate" />
        <div v-if="previewTemplate" class="flex justify-end gap-2">
          <Button v-if="canWrite" variant="outline" size="sm" @click="duplicate(previewTemplate)"><CopyPlus class="h-4 w-4" />{{ $t('templates.duplicate') }}</Button>
          <RouterLink :to="`/templates/${previewTemplate.id}`"><Button size="sm"><Pencil class="h-4 w-4" />{{ $t('common.edit') }}</Button></RouterLink>
        </div>
      </DialogContent>
    </Dialog>

    <TemplateApiDialog v-model:open="apiOpen" :template="apiTemplate" />

    <!-- Delete Confirmation Dialog -->
    <DeleteConfirmDialog
      v-model:open="deleteDialogOpen"
      :title="$t('templates.deleteTemplate')"
      :item-name="templateToDelete?.display_name || templateToDelete?.name"
      :is-submitting="isDeleting"
      @confirm="confirmDeleteTemplate"
    />
  </div>
</template>
