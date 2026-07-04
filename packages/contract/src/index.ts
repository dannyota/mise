export const VERSION = '0.2.0';

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

export type {
  ApiProblem,
  BootstrapResponse,
  CorpusStatus,
  DashboardSummary,
  CursorPage,
  ReviewCandidate,
  Finding,
  FindingDetail,
  Resolution,
  TimelineEvent,
  Notification,
  Webhook,
  TranslateResponse,
  CorpusAdmin,
  RestGraphNode,
  RestGraphEdge,
  GraphResponse,
  ChainHop,
  ChainResponse,
  CorpusDescriptor,
  RegistryListResponse,
} from './rest-types.js';
