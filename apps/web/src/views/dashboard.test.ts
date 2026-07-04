import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { nextTick } from 'vue';
import DashboardView from './DashboardView.vue';
import type { DashboardSummary } from '@mise/contract';

const summary: DashboardSummary = {
  coverage_pct: 87,
  open_conflicts: 5,
  staleness_alerts: 2,
  review_queue_depth: 3,
  corpora: [
    {
      corpus_id: 'vn-law',
      name: 'Vietnam Law',
      status: 'healthy',
      document_count: 30,
      last_ingest: '2026-07-01T00:00:00Z',
    },
  ],
};

describe('DashboardView', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });
  afterEach(() => vi.restoreAllMocks());

  it('renders summary tiles after fetch', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(summary)),
    );
    const w = mount(DashboardView);
    await flushPromises();
    await nextTick();
    await flushPromises();
    expect(w.text()).toContain('87%');
    expect(w.text()).toContain('Coverage');
    expect(w.text()).toContain('vn-law');
    expect(w.text()).toContain('healthy');
  });

  it('renders error on failure', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('Network error'));
    const w = mount(DashboardView);
    await flushPromises();
    await nextTick();
    await flushPromises();
    expect(w.text()).toContain('Network error');
  });
});
