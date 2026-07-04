import { describe, expect, it } from 'vitest';
import { isToolAllowed, ALLOWED_TOOL_NAMES } from './permissions.js';

describe('isToolAllowed', () => {
  it.each(ALLOWED_TOOL_NAMES)('allows %s', (tool) => {
    expect(isToolAllowed(tool)).toBe(true);
  });

  it('denies filesystem tool', () => {
    expect(isToolAllowed('filesystem__read')).toBe(false);
  });

  it('denies bash tool', () => {
    expect(isToolAllowed('bash__run')).toBe(false);
  });

  it('denies computer tool', () => {
    expect(isToolAllowed('computer__screenshot')).toBe(false);
  });

  it('denies text_editor tool', () => {
    expect(isToolAllowed('text_editor__write')).toBe(false);
  });

  it('denies unknown tool', () => {
    expect(isToolAllowed('unknown_tool')).toBe(false);
  });

  it('denies arbitrary MCP tool not in allowlist', () => {
    expect(isToolAllowed('mcp__other__write')).toBe(false);
  });
});
