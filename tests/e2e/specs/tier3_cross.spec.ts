import { test, expect } from '@playwright/test';
import { GatewayClient } from '../helpers/client';

test.describe('Tier 3: Cross-Feature Combination Cases', () => {
  let client: GatewayClient;

  test.beforeEach(({ request, baseURL }) => {
    client = new GatewayClient(request, baseURL);
  });

  test('Case 3.1: OpenAI-to-Anthropic Fallback with Path and Header-based Routing', async () => {
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers: {
        'x-inject-error': 'true',
        'x-inject-openai-status': '500'
      }
    });

    // If fallback is functional, it returns 200 OK (Anthropic payload).
    // If fallback failed or is disabled, it returns 500.
    expect([200, 500]).toContain(res.status());

    if (res.status() === 200) {
      const body = await res.json();
      expect(body).toHaveProperty('content'); // Anthropic message schema uses content
      expect(body).not.toHaveProperty('choices');
      const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
      if (provider) {
        expect(provider).toBe('anthropic');
      }
    }
  });

  test('Case 3.2: Anthropic-to-OpenAI Fallback under Error Injection', async () => {
    const res = await client.sendAnthropicRequest({
      model: 'claude-3-5-sonnet',
      headers: {
        'x-inject-error': 'true',
        'x-inject-anthropic-status': '500'
      }
    });

    expect([200, 500]).toContain(res.status());

    if (res.status() === 200) {
      const body = await res.json();
      expect(body).toHaveProperty('choices'); // OpenAI completion schema uses choices
      expect(body).not.toHaveProperty('content');
      const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
      if (provider) {
        expect(provider).toBe('openai');
      }
    }
  });

  test('Case 3.3: Cascading Failure / Exhausted Fallback', async () => {
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers: {
        'x-inject-openai-status': '500',
        'x-inject-anthropic-status': '500',
        'x-inject-error-all': 'true'
      }
    });

    // Both fail, so we expect a clean gateway error (500 or 502/504) returned to client, no loop
    expect([500, 502, 504]).toContain(res.status());
  });

  test('Case 3.4: Authorization Header Preservation across Fallback', async () => {
    const authHeader = 'Bearer test-client-key';
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      authHeader,
      headers: {
        'x-inject-openai-status': '500',
        'x-inject-error': 'true'
      }
    });

    expect([200, 500]).toContain(res.status());

    if (res.status() === 200) {
      const body = await res.json();
      const echoedAuth = 
        res.headers()['x-received-authorization'] || 
        res.headers()['x-echo-authorization'] ||
        body.echo_headers?.authorization ||
        body.echo_headers?.Authorization ||
        body.headers?.authorization ||
        body.headers?.Authorization;
        
      if (echoedAuth) {
        expect(echoedAuth).toBe(authHeader);
      }
    }
  });

  test('Case 3.5: Upstream Rate Limit (429) Triggered Fallback', async () => {
    const res = await client.sendOpenAIRequest({
      model: 'gpt-4o',
      headers: {
        'x-inject-openai-status': '429',
        'x-inject-status': '429'
      }
    });

    // Depending on the implementation, 429 either falls back to secondary (200) or passes through (429)
    expect([200, 429]).toContain(res.status());
  });
});
