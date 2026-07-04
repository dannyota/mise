import { describe, expect, it } from 'vitest';
import { parseSseStream } from './sse.js';

function makeResponse(text: string): Response {
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(text));
      controller.close();
    },
  });
  return new Response(stream);
}

describe('parseSseStream', () => {
  it('parses token events', async () => {
    const res = makeResponse(
      'event:token\ndata:hello\n\nevent:token\ndata: world\n\n',
    );
    const events: { event: string; data: string }[] = [];
    for await (const e of parseSseStream(res)) {
      events.push(e);
    }
    expect(events).toEqual([
      { event: 'token', data: 'hello' },
      { event: 'token', data: 'world' },
    ]);
  });

  it('parses citation events', async () => {
    const res = makeResponse('event:citation\ndata:art-10\n\n');
    const events: { event: string; data: string }[] = [];
    for await (const e of parseSseStream(res)) {
      events.push(e);
    }
    expect(events).toEqual([{ event: 'citation', data: 'art-10' }]);
  });
});
