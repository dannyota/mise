import { createRouter, createWebHistory } from 'vue-router';

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'health',
      component: () => import('@/views/HealthView.vue'),
    },
  ],
});

export default router;
