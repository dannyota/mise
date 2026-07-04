import { z } from 'zod';

export const chatRequestSchema = z.object({
  question: z.string().min(1).max(2000),
  corpora: z.array(z.string()).optional(),
  locale: z.enum(['en', 'vi', 'ms']).default('en'),
  idempotencyKey: z.string().uuid().optional(),
});

export type ChatRequest = z.infer<typeof chatRequestSchema>;

export function parseChatRequest(body: unknown): ChatRequest {
  return chatRequestSchema.parse(body);
}
