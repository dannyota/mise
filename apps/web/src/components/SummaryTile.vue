<script setup lang="ts">
import type { Component } from 'vue';

defineProps<{
  label: string;
  value: string | number;
  icon: Component;
}>();
</script>

<template>
  <div class="rounded border bg-white p-4">
    <div class="flex items-center gap-3">
      <div class="rounded-lg bg-violet-50 p-2 text-violet-700">
        <component :is="icon" :size="20" />
      </div>
      <div>
        <p class="text-2xl font-bold">{{ value }}</p>
        <p class="text-xs text-gray-500">{{ label }}</p>
      </div>
    </div>
  </div>
</template>
