<script setup lang="ts">
import { Button } from '@/components/ui/button'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { ArrowLeft } from 'lucide-vue-next'
import type { Component } from 'vue'

defineProps<{
  title: string
  description?: string
  icon?: Component
  backLink?: string
  breadcrumbs?: Array<{ label: string; href?: string }>
}>()
</script>

<template>
  <header class="border-b border-white/[0.08] light:border-gray-200 bg-[#0a0a0b]/95 light:bg-white/95 backdrop-blur">
    <div class="flex min-h-16 flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3 md:flex-nowrap md:px-6">
      <RouterLink v-if="backLink" :to="backLink" class="-mr-1 shrink-0" :aria-label="$t('common.back')">
        <Button variant="ghost" size="icon-sm">
          <ArrowLeft class="h-4 w-4" />
        </Button>
      </RouterLink>
      <div
        v-if="icon"
        class="h-8 w-8 shrink-0 rounded-lg flex items-center justify-center border border-white/[0.08] bg-white/[0.04] light:border-gray-200 light:bg-gray-50"
      >
        <component :is="icon" class="h-4 w-4 text-white/70 light:text-gray-600" />
      </div>
      <div class="flex-1 min-w-[10rem]">
        <h1 class="text-lg font-semibold leading-tight truncate text-white light:text-gray-900">{{ title }}</h1>
        <template v-if="breadcrumbs?.length">
          <Breadcrumb>
            <BreadcrumbList>
              <template v-for="(crumb, index) in breadcrumbs" :key="index">
                <BreadcrumbItem>
                  <BreadcrumbLink v-if="crumb.href" :href="crumb.href">
                    {{ crumb.label }}
                  </BreadcrumbLink>
                  <BreadcrumbPage v-else>{{ crumb.label }}</BreadcrumbPage>
                </BreadcrumbItem>
                <BreadcrumbSeparator v-if="index < breadcrumbs.length - 1" />
              </template>
            </BreadcrumbList>
          </Breadcrumb>
        </template>
        <p v-else-if="description" class="text-sm text-white/50 light:text-gray-500 truncate">
          {{ description }}
        </p>
      </div>
      <div v-if="$slots.actions" class="flex w-full flex-wrap items-center gap-2 md:w-auto md:shrink-0 md:flex-nowrap">
        <slot name="actions" />
      </div>
    </div>
  </header>
</template>
