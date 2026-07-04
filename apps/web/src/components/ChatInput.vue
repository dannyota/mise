<script setup lang="ts">
import { ref } from 'vue';
import { Send } from 'lucide-vue-next';

const emit = defineEmits<{ submit: [question: string] }>();
defineProps<{ disabled?: boolean }>();

const input = ref('');

function handleSubmit(): void {
  const q = input.value.trim();
  if (!q) return;
  emit('submit', q);
  input.value = '';
}
</script>

<template>
  <form class="flex gap-2" @submit.prevent="handleSubmit">
    <input
      v-model="input"
      type="text"
      placeholder="Ask about regulatory evidence..."
      class="flex-1 rounded border px-3 py-2 text-sm focus:border-violet-500 focus:outline-none"
      :disabled="disabled"
    />
    <button
      type="submit"
      :disabled="disabled || !input.trim()"
      class="rounded bg-violet-700 px-3 py-2 text-white disabled:opacity-50"
    >
      <Send :size="16" />
    </button>
  </form>
</template>
