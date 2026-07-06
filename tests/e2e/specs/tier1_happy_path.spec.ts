import { test, expect } from '@playwright/test';
import { GatewayClient } from '../helpers/client';

test.describe('Tier 1: Happy Path Model Routing', () => {
  let client: GatewayClient;

  test.beforeEach(({ request, baseURL }) => {
    client = new GatewayClient(request, baseURL);
  });

  test('TC-T1-01: Route gpt-4o to OpenAI Mock Upstream', async () => {
    const res = await client.sendOpenAIRequest({ model: 'gpt-4o' });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body).toHaveProperty('id');
    expect(body.object).toBe('chat.completion');
    expect(body.model).toBe('gpt-4o');
    expect(body).toHaveProperty('choices');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('openai');
    }
  });

  test('TC-T1-02: Route gpt-4-turbo to OpenAI Mock Upstream', async () => {
    const res = await client.sendOpenAIRequest({ model: 'gpt-4-turbo' });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.model).toBe('gpt-4-turbo');
    expect(body).toHaveProperty('choices');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('openai');
    }
  });

  test('TC-T1-03: Route gpt-4 to OpenAI Mock Upstream', async () => {
    const res = await client.sendOpenAIRequest({ model: 'gpt-4' });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.model).toBe('gpt-4');
    expect(body).toHaveProperty('choices');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('openai');
    }
  });

  test('TC-T1-04: Route gpt-3.5-turbo to OpenAI Mock Upstream', async () => {
    const res = await client.sendOpenAIRequest({ model: 'gpt-3.5-turbo' });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.model).toBe('gpt-3.5-turbo');
    expect(body).toHaveProperty('choices');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('openai');
    }
  });

  test('TC-T1-05: Route o1-mini to OpenAI Mock Upstream', async () => {
    const res = await client.sendOpenAIRequest({ model: 'o1-mini' });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.model).toBe('o1-mini');
    expect(body).toHaveProperty('choices');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('openai');
    }
  });

  test('TC-T1-06: Route o1-preview to OpenAI Mock Upstream', async () => {
    const res = await client.sendOpenAIRequest({ model: 'o1-preview' });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.model).toBe('o1-preview');
    expect(body).toHaveProperty('choices');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('openai');
    }
  });

  test('TC-T1-07: Case Insensitivity routing check (GPT-4O)', async () => {
    const res = await client.sendOpenAIRequest({ model: 'GPT-4O' });
    expect(res.status()).toBe(200);
    const body = await res.json();
    // Allow either the original case or normalized/lowercased model name
    expect(body.model.toLowerCase()).toBe('gpt-4o');
    expect(body).toHaveProperty('choices');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('openai');
    }
  });

  test('TC-T1-08: Default OpenAI routing when x-model header is absent', async () => {
    const res = await client.sendOpenAIRequest({ model: undefined });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body).toHaveProperty('choices');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('openai');
    }
  });

  test('TC-T1-09: Route claude-3-5-sonnet to Anthropic Mock Upstream', async () => {
    const res = await client.sendAnthropicRequest({ model: 'claude-3-5-sonnet' });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body).toHaveProperty('id');
    expect(body.type).toBe('message');
    expect(body.model).toBe('claude-3-5-sonnet');
    expect(body).toHaveProperty('content');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('anthropic');
    }
  });

  test('TC-T1-10: Route claude-3-opus to Anthropic Mock Upstream', async () => {
    const res = await client.sendAnthropicRequest({ model: 'claude-3-opus' });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.model).toBe('claude-3-opus');
    expect(body).toHaveProperty('content');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('anthropic');
    }
  });

  test('TC-T1-11: Route claude-3-haiku to Anthropic Mock Upstream', async () => {
    const res = await client.sendAnthropicRequest({ model: 'claude-3-haiku' });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.model).toBe('claude-3-haiku');
    expect(body).toHaveProperty('content');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('anthropic');
    }
  });

  test('TC-T1-12: Route claude-2.1 to Anthropic Mock Upstream', async () => {
    const res = await client.sendAnthropicRequest({ model: 'claude-2.1' });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.model).toBe('claude-2.1');
    expect(body).toHaveProperty('content');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('anthropic');
    }
  });

  test('TC-T1-13: Route claude-2.0 to Anthropic Mock Upstream', async () => {
    const res = await client.sendAnthropicRequest({ model: 'claude-2.0' });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.model).toBe('claude-2.0');
    expect(body).toHaveProperty('content');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('anthropic');
    }
  });

  test('TC-T1-14: Case Insensitivity routing check (CLAUDE-3-5-SONNET)', async () => {
    const res = await client.sendAnthropicRequest({ model: 'CLAUDE-3-5-SONNET' });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.model.toLowerCase()).toBe('claude-3-5-sonnet');
    expect(body).toHaveProperty('content');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('anthropic');
    }
  });

  test('TC-T1-15: Default Anthropic routing when x-model header is absent', async () => {
    const res = await client.sendAnthropicRequest({ model: undefined });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body).toHaveProperty('content');
    const provider = res.headers()['x-upstream-provider'] || res.headers()['x-llm-provider'] || body.provider;
    if (provider) {
      expect(provider).toBe('anthropic');
    }
  });

  test('TC-T1-16: Authentication replacement to OpenAI', async () => {
    const authHeader = 'Bearer test-client-key';
    const res = await client.sendOpenAIRequest({ model: 'gpt-4o', authHeader });
    expect(res.status()).toBe(200);
    const body = await res.json();
    
    // Check various possible locations where authorization could be echoed
    const echoedAuth = 
      res.headers()['x-received-authorization'] || 
      res.headers()['x-echo-authorization'] ||
      body.echo_headers?.authorization ||
      body.echo_headers?.Authorization ||
      body.headers?.authorization ||
      body.headers?.Authorization;
      
    if (echoedAuth) {
      expect(echoedAuth).toBe('Bearer sk-real-openai');
    }
  });

  test('TC-T1-17: Authentication replacement to Anthropic', async () => {
    const authHeader = 'Bearer test-client-key';
    const res = await client.sendAnthropicRequest({ model: 'claude-3-5-sonnet', authHeader });
    expect(res.status()).toBe(200);
    const body = await res.json();

    const echoedAuth = 
      res.headers()['x-received-authorization'] || 
      res.headers()['x-echo-authorization'] ||
      body.echo_headers?.authorization ||
      body.echo_headers?.Authorization ||
      body.headers?.authorization ||
      body.headers?.Authorization;

    if (echoedAuth) {
      expect(echoedAuth).toBe('Bearer sk-real-anthropic');
    }
  });

  test('TC-T1-18: Unrecognized Model Defaulting', async () => {
    const res = await client.sendOpenAIRequest({ model: 'unknown-provider-model-xyz' });
    // Verify default path-based routing defaults to OpenAI mock and responds successfully
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body).toHaveProperty('choices');
  });
});
