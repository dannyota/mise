<script setup lang="ts">
import type { CorpusStatus } from '@mise/contract';

defineProps<{
  corpora: CorpusStatus[];
}>();
</script>

<template>
  <div class="rounded border bg-white">
    <h3 class="border-b px-4 py-3 text-sm font-semibold">Corpus Status</h3>
    <div class="divide-y">
      <div
        v-for="c in corpora"
        :key="c.corpus_id"
        class="flex items-center justify-between px-4 py-2 text-sm"
      >
        <span>{{ c.corpus_id }}</span>
        <span
          class="rounded px-2 py-0.5 text-xs"
          :class="{
            'bg-green-100 text-green-700': c.status === 'healthy',
            'bg-yellow-100 text-yellow-700': c.status === 'ingesting',
            'bg-red-100 text-red-700': c.status === 'error',
          }"
        >
          {{ c.status }}
        </span>
      </div>
    </div>
  </div>
</template>
