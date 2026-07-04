<script setup lang="ts">
import type { TimelineEvent } from '@mise/contract';

defineProps<{ events: TimelineEvent[] }>();
</script>

<template>
  <div class="space-y-3">
    <div
      v-for="(evt, i) in events"
      :key="i"
      class="flex gap-3 border-l-2 border-violet-200 pl-4"
    >
      <div>
        <p class="text-sm font-medium">{{ evt.action }}</p>
        <p class="text-xs text-gray-500">{{ evt.actor }} &middot; {{ evt.timestamp }}</p>
        <p v-if="evt.detail" class="mt-1 text-sm text-gray-600">{{ evt.detail }}</p>
      </div>
    </div>
  </div>
</template>
