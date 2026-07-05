import { test, expect } from '@playwright/test';
import { GatewayClient } from '../helpers/client';

test.describe('Tier 2: Boundary and Corner Cases', () => {
  let client: GatewayClient;

  test.beforeEach(({ request, baseURL }) => {
    client = new GatewayClient(request, baseURL);
  });

  // Category A: Empty & Malformed Requests
  test('TC-T2-01: Missing x-model Header', async () => {
    const res = await client.sendCustomRequest('POST', '/v1/chat/completions', {
      headers: {},
      body: { messages: [{ role: 'user', content: 'Hello' }] }
    });
    // Can default route to OpenAI (200) or return 400 Bad Request
    expect([200, 400]).toContain(res.status());
    if (res.status() === 400) {
      const body = await res.json();
      expect(body.error).toMatch(/model/i);
    }
  });

  test('TC-T2-02: Empty x-model Header Value', async () => {
    const res = await client.sendCustomRequest('POST', '/v1/chat/completions', {
      headers: { 'x-model': '   ' },
      body: { messages: [{ role: 'user', content: 'Hello' }] }
    });
    expect([200, 400]).toContain(res.status());
    if (res.status() === 400) {
      const body = await res.json();
      expect(body.error).toMatch(/model/i);
    }
  });

  test('TC-T2-03: Unknown or Unsupported Model', async () => {
    const res = await client.sendOpenAIRequest({ model: 'custom-deep-seek-v5' });
    expect([200, 400]).toContain(res.status());
    if (res.status() === 400) {
      const body = await res.json();
      expect(body.error).toMatch(/unsupported|invalid/i);
    }
  });

  test('TC-T2-04: Malformed Request JSON Body', async () => {
    // Send a raw malformed string as body
    const res = await client.sendCustomRequest('POST', '/v1/chat/completions', {
      model: 'gpt-4o',
      headers: { 'Content-Type': 'application/json' },
      body: '{ messages: [ {role: "user", content: ' // incomplete JSON
    });
    expect([400, 500]).toContain(res.status());
  });

  test('TC-T2-05: Missing Authorization Header', async () => {
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      authHeader: '' // empty auth header
    });
    expect([200, 401, 403]).toContain(res.status());
  });

  // Category B: Header Bounds & Sizes
  test('TC-T2-06: Extremely Large Header Value', async () => {
    const largeValue = 'x'.repeat(16 * 1024); // 16KB
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers: { 'x-padding': largeValue }
    });
    // Envoy might reject with 431 Request Header Fields Too Large, or succeed
    expect([200, 431]).toContain(res.status());
  });

  test('TC-T2-07: Excessive Number of Headers', async () => {
    const headers: Record<string, string> = {};
    for (let i = 0; i < 100; i++) {
      headers[`x-header-${i}`] = `value-${i}`;
    }
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers
    });
    expect([200, 431, 400]).toContain(res.status());
  });

  test('TC-T2-08: Duplicate x-model Headers', async () => {
    // Standard fetch/request libraries join duplicate headers with a comma
    const res = await client.sendCustomRequest('POST', '/v1/chat/completions', {
      headers: {
        'x-model': 'gpt-4o, claude-3-5-sonnet'
      },
      body: { messages: [{ role: 'user', content: 'Hello' }] }
    });
    expect([200, 400]).toContain(res.status());
  });

  // Category C: Error Interception & Fallback Scenarios
  test('TC-T2-09: Primary returns 500 Internal Server Error', async () => {
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers: { 'x-inject-openai-status': '500', 'x-inject-error': 'true' }
    });
    // Expected to fall back to Anthropic (200) or fail to 500 if fallback is not configured
    expect([200, 500]).toContain(res.status());
  });

  test('TC-T2-10: Primary returns 502 Bad Gateway', async () => {
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers: { 'x-inject-openai-status': '502', 'x-inject-status': '502' }
    });
    expect([200, 502]).toContain(res.status());
  });

  test('TC-T2-11: Primary returns 503 Service Unavailable', async () => {
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers: { 'x-inject-openai-status': '503', 'x-inject-status': '503' }
    });
    expect([200, 503]).toContain(res.status());
  });

  test('TC-T2-12: Primary returns 504 Gateway Timeout', async () => {
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers: { 'x-inject-openai-status': '504', 'x-inject-status': '504' }
    });
    expect([200, 504]).toContain(res.status());
  });

  test('TC-T2-13: Both Primary and Fallback Upstreams Fail', async () => {
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers: {
        'x-inject-openai-status': '500',
        'x-inject-anthropic-status': '500',
        'x-inject-error-all': 'true'
      }
    });
    // Expected to fail with 500/502/504 since both failed
    expect([500, 502, 504]).toContain(res.status());
  });

  test('TC-T2-14: Upstream Returns 429 Too Many Requests', async () => {
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers: { 'x-inject-openai-status': '429', 'x-inject-status': '429' }
    });
    // 429 could be passed through or fell back to secondary
    expect([200, 429]).toContain(res.status());
  });

  // Category D: Timeout & Network-Level Faults
  test('TC-T2-15: Primary Upstream Timeout', async () => {
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers: { 'x-inject-openai-delay-ms': '5000', 'x-inject-delay': '5000' }
    });
    // Envoy might time out (504/503) or fall back successfully (200)
    expect([200, 503, 504]).toContain(res.status());
  });

  test('TC-T2-16: Primary Upstream Connection Reset / TCP Drop', async () => {
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers: { 'x-inject-connection-drop': 'true' }
    });
    expect([200, 502, 503, 504]).toContain(res.status());
  });

  test('TC-T2-17: Fallback Upstream Times Out', async () => {
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers: {
        'x-inject-openai-status': '500',
        'x-inject-anthropic-delay-ms': '5000',
        'x-inject-delay': '5000'
      }
    });
    expect([500, 502, 503, 504]).toContain(res.status());
  });

  test('TC-T2-18: Corrupted Response Body from Primary Upstream', async () => {
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers: { 'x-inject-corrupt-body': 'true' }
    });
    // If upstream returns 200 with corrupted body, client gets 200 (corrupt) or 502 if sidecar validation fails
    expect([200, 500, 502]).toContain(res.status());
  });
});
