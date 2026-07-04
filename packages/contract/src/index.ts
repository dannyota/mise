export const VERSION = '0.1.0';

export type {
  SseEvent,
  SseTokenEvent,
  SseCitationEvent,
  SseChainEvent,
  SseEvidenceCheckedEvent,
  SseAbstainEvent,
  SseDoneEvent,
  SseErrorEvent,
} from './sse-events.js';

export { SSE_EVENT_TYPES } from './sse-events.js';

export type {
  SearchInput,
  SearchOutput,
  SectionHit,
  DocumentInput,
  DocumentOutput,
  GraphInput,
  GraphOutput,
} from './mcp-schemas.js';
