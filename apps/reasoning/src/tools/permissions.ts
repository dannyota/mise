const ALLOWED_TOOLS = new Set([
  'mcp__mise__search',
  'mcp__mise__document',
  'mcp__mise__graph',
]);

const DENIED_PREFIXES = [
  'filesystem__',
  'bash__',
  'computer__',
  'text_editor__',
];

export function isToolAllowed(toolName: string): boolean {
  if (ALLOWED_TOOLS.has(toolName)) {
    return true;
  }
  for (const prefix of DENIED_PREFIXES) {
    if (toolName.startsWith(prefix)) {
      return false;
    }
  }
  return false;
}

export const ALLOWED_TOOL_NAMES = [...ALLOWED_TOOLS] as const;
