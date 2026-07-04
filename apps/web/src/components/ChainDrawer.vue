<script setup lang="ts">
import { X } from 'lucide-vue-next';
import type { ChainHop } from '@mise/contract';

defineProps<{ hops: readonly ChainHop[]; open: boolean }>();
const emit = defineEmits<{ close: [] }>();
</script>

<template>
  <Transition name="slide">
    <aside
      v-if="open"
      class="absolute right-0 top-0 z-10 flex h-full w-80 flex-col border-l bg-white shadow-lg"
    >
      <div class="flex items-center justify-between border-b px-4 py-3">
        <h3 class="text-sm font-semibold">Derivation Chain</h3>
        <button class="rounded p-1 hover:bg-gray-100" @click="emit('close')">
          <X :size="16" />
        </button>
      </div>
      <div class="flex-1 overflow-y-auto p-4">
        <ol class="space-y-3">
          <li
            v-for="(hop, i) in hops"
            :key="i"
            class="rounded border p-3 text-sm"
          >
            <p class="font-medium">{{ hop.citation }}</p>
            <p class="mt-1 text-xs text-gray-500">
              {{ hop.corpus_id }} &middot; {{ hop.edge_type }} &middot;
              {{ (hop.confidence * 100).toFixed(0) }}%
            </p>
          </li>
        </ol>
      </div>
    </aside>
  </Transition>
</template>
