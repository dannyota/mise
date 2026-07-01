<script setup lang="ts">
import { ref, onMounted } from 'vue';

const status = ref('checking...');

onMounted(async () => {
  try {
    const res = await fetch('/api/healthz');
    status.value = res.ok ? 'healthy' : 'unhealthy';
  } catch {
    status.value = 'unreachable';
  }
});
</script>

<template>
  <div>
    <h1>mise</h1>
    <p>serving: {{ status }}</p>
  </div>
</template>
