<script setup lang="ts">
import { ref } from 'vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { PageHeader } from '@/components/shared'
import TransactionsTable from './TransactionsTable.vue'
import { useWalletStore } from '@/stores/wallet'
import { Receipt } from 'lucide-vue-next'

const walletStore = useWalletStore()
const currency = ref(walletStore.currency)
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('owner.transactionsTitle')" :description="$t('owner.transactionsSubtitle')" :icon="Receipt" icon-gradient="bg-gradient-to-br from-sky-500 to-blue-600 shadow-sky-500/20" />
    <ScrollArea class="flex-1">
      <div class="p-6 max-w-7xl mx-auto">
        <Card>
          <CardHeader>
            <CardTitle>{{ $t('owner.allTransactions') }}</CardTitle>
            <CardDescription>{{ $t('owner.allTransactionsDesc') }}</CardDescription>
          </CardHeader>
          <CardContent>
            <TransactionsTable scope="all" :currency="currency" />
          </CardContent>
        </Card>
      </div>
    </ScrollArea>
  </div>
</template>
