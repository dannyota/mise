<script setup lang="ts">
import { ref } from 'vue';

const emit = defineEmits<{ submit: [note: string] }>();
const note = ref('');

function handleSubmit(): void {
  const n = note.value.trim();
  if (!n) return;
  emit('submit', n);
  note.value = '';
}
</script>

<template>
  <form class="flex gap-2" @submit.prevent="handleSubmit">
    <textarea
      v-model="note"
      placeholder="Resolution note..."
      rows="2"
      class="flex-1 rounded border px-3 py-2 text-sm focus:border-violet-500 focus:outline-none"
    />
    <button
      type="submit"
      :disabled="!note.trim()"
      class="self-end rounded bg-violet-700 px-4 py-2 text-sm text-white disabled:opacity-50"
    >
      Resolve
    </button>
  </form>
</template>
