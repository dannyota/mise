import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import LoadingState from './LoadingState.vue';
import EmptyState from './EmptyState.vue';
import ErrorBanner from './ErrorBanner.vue';
import { ApiClientError } from '@/api/client.js';

describe('LoadingState', () => {
  it('renders spinner with sr-only text', () => {
    const w = mount(LoadingState);
    expect(w.text()).toContain('Loading...');
    expect(w.find('[role="status"]').exists()).toBe(true);
  });
});

describe('EmptyState', () => {
  it('renders message', () => {
    const w = mount(EmptyState, {
      props: { message: 'No findings' },
    });
    expect(w.text()).toContain('No findings');
  });
});

describe('ErrorBanner', () => {
  it('renders plain error', () => {
    const w = mount(ErrorBanner, {
      props: { error: new Error('Something failed') },
    });
    expect(w.text()).toContain('Something failed');
  });

  it('renders ApiClientError with detail', () => {
    const err = new ApiClientError({
      type: 'about:blank',
      title: 'Not Found',
      status: 404,
      detail: 'Resource does not exist',
    });
    const w = mount(ErrorBanner, { props: { error: err } });
    expect(w.text()).toContain('Not Found');
    expect(w.text()).toContain('Resource does not exist');
  });
});
