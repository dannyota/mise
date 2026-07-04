import { beforeEach, describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { createRouter, createMemoryHistory } from 'vue-router';
import SidebarNav from './SidebarNav.vue';

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div/>' } },
      {
        path: '/graph',
        component: { template: '<div/>' },
      },
      {
        path: '/review',
        component: { template: '<div/>' },
      },
      {
        path: '/findings',
        component: { template: '<div/>' },
      },
      {
        path: '/chat',
        component: { template: '<div/>' },
      },
      {
        path: '/timeline',
        component: { template: '<div/>' },
      },
      {
        path: '/notifications',
        component: { template: '<div/>' },
      },
      {
        path: '/reports',
        component: { template: '<div/>' },
      },
      {
        path: '/admin',
        component: { template: '<div/>' },
      },
    ],
  });
}

describe('SidebarNav', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('renders all nav links', async () => {
    const router = makeRouter();
    await router.push('/');
    await router.isReady();
    const wrapper = mount(SidebarNav, {
      global: { plugins: [router] },
    });
    expect(wrapper.text()).toContain('Dashboard');
    expect(wrapper.text()).toContain('Graph');
    expect(wrapper.text()).toContain('Review');
    expect(wrapper.text()).toContain('Findings');
    expect(wrapper.text()).toContain('Audit Q&A');
    expect(wrapper.text()).toContain('Admin');
  });
});
