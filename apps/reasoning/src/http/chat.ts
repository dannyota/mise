import { Hono } from 'hono';
import { streamSSE } from 'hono/streaming';
import { getCaller } from '../auth/caller.js';
import { createMcpClient } from '../tools/mcp-client.js';
import { parseChatRequest } from './request-envelope.js';
import { runAgentLoop } from '../agent/loop.js';
import type { AgentResult, ModelCall } from '../agent/loop.js';
import type { Config } from '../config.js';

type SSEWriter = {
  writeSSE(event: { event: string; data: string }): Promise<void>;
};

export function chatRoute(config: Config, modelCall: ModelCall): Hono {
  const app = new Hono();

  app.post('/', async (c) => {
    let body: unknown;
    try {
      body = await c.req.json();
    } catch {
      return c.json(
        { type: 'bad_request', title: 'Invalid JSON', status: 400 },
        400,
      );
    }

    let request;
    try {
      request = parseChatRequest(body);
    } catch (err) {
      return c.json(
        {
          type: 'validation_error',
          title: 'Invalid request',
          status: 422,
          detail: String(err),
        },
        422,
      );
    }

    const caller = getCaller(c);
    const mcp = createMcpClient(config, caller);
    const controller = new AbortController();

    return streamSSE(c, async (stream) => {
      stream.onAbort(() => controller.abort());
      try {
        const result = await runAgentLoop({
          question: request.question,
          corpora: request.corpora,
          mcp,
          config,
          modelCall,
          signal: controller.signal,
        });
        await emitResult(stream, result);
      } catch (err) {
        if (!controller.signal.aborted) {
          await stream.writeSSE({
            event: 'error',
            data: JSON.stringify({
              type: 'agent_error',
              detail: err instanceof Error ? err.message : 'Unknown error',
            }),
          });
        }
      }
    });
  });

  return app;
}

async function emitResult(stream: SSEWriter, result: AgentResult): Promise<void> {
  for (const citation of result.citations) {
    await stream.writeSSE({
      event: 'evidence_checked',
      data: JSON.stringify({
        corpus_id: citation.corpusId,
        citation_path: citation.citationPath,
        source_url: citation.sourceUrl,
      }),
    });
  }

  for (const hop of result.chain) {
    await stream.writeSSE({ event: 'chain', data: JSON.stringify(hop) });
  }

  if (result.kind === 'abstain') {
    await stream.writeSSE({
      event: 'abstain',
      data: JSON.stringify({ reason: result.text }),
    });
  } else {
    for (const chunk of splitChunks(result.text, 100)) {
      await stream.writeSSE({
        event: 'token',
        data: JSON.stringify({ text: chunk }),
      });
    }
    for (const [i, citation] of result.citations.entries()) {
      await stream.writeSSE({
        event: 'citation',
        data: JSON.stringify({
          index: i + 1,
          corpus_id: citation.corpusId,
          document_id: citation.documentId,
          citation_path: citation.citationPath,
          source_url: citation.sourceUrl,
        }),
      });
    }
  }

  await stream.writeSSE({
    event: 'done',
    data: JSON.stringify({ model: result.model, iterations: result.iterations }),
  });
}

function splitChunks(text: string, size: number): string[] {
  const chunks: string[] = [];
  for (let i = 0; i < text.length; i += size) {
    chunks.push(text.slice(i, i + size));
  }
  return chunks;
}
