import { APIRequestContext, APIResponse } from '@playwright/test';

export interface RequestOptions {
  model?: string;
  authHeader?: string;
  headers?: Record<string, string>;
  body?: any;
}

export class GatewayClient {
  private request: APIRequestContext;
  private baseURL: string;

  constructor(request: APIRequestContext, baseURL = 'http://localhost:8080') {
    this.request = request;
    this.baseURL = baseURL;
  }

  /**
   * Sends a request to the OpenAI endpoint (/v1/chat/completions)
   */
  async sendOpenAIRequest(opts: RequestOptions = {}): Promise<APIResponse> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...opts.headers,
    };
    if (opts.model !== undefined) {
      headers['x-model'] = opts.model;
    }
    if (opts.authHeader !== undefined) {
      if (opts.authHeader) {
        headers['Authorization'] = opts.authHeader;
      }
    } else {
      headers['Authorization'] = 'Bearer default-openai-token';
    }

    const body = opts.body !== undefined ? opts.body : {
      model: opts.model || 'gpt-4o',
      messages: [{ role: 'user', content: 'Hello, this is a test message.' }]
    };

    return await this.request.post(`${this.baseURL}/v1/chat/completions`, {
      headers,
      data: body,
    });
  }

  /**
   * Sends a request to the Anthropic endpoint (/v1/messages)
   */
  async sendAnthropicRequest(opts: RequestOptions = {}): Promise<APIResponse> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...opts.headers,
    };
    if (opts.model !== undefined) {
      headers['x-model'] = opts.model;
    }
    if (opts.authHeader !== undefined) {
      if (opts.authHeader) {
        headers['Authorization'] = opts.authHeader;
      }
    } else {
      headers['Authorization'] = 'Bearer default-anthropic-token';
    }

    const body = opts.body !== undefined ? opts.body : {
      model: opts.model || 'claude-3-5-sonnet',
      max_tokens: 1024,
      messages: [{ role: 'user', content: 'Hello, this is a test message.' }]
    };

    return await this.request.post(`${this.baseURL}/v1/messages`, {
      headers,
      data: body,
    });
  }

  /**
   * Generic request method for boundary, invalid paths or custom methods.
   */
  async sendCustomRequest(method: 'GET' | 'POST' | 'PUT' | 'DELETE', path: string, opts: RequestOptions = {}): Promise<APIResponse> {
    const headers: Record<string, string> = {
      ...opts.headers,
    };
    if (opts.model !== undefined) {
      headers['x-model'] = opts.model;
    }
    if (opts.authHeader !== undefined) {
      if (opts.authHeader) {
        headers['Authorization'] = opts.authHeader;
      }
    } else if (headers['Authorization'] === undefined && opts.authHeader === undefined) {
      headers['Authorization'] = 'Bearer default-token';
    }

    return await this.request[method.toLowerCase() as 'get' | 'post' | 'put' | 'delete'](`${this.baseURL}${path}`, {
      headers,
      data: opts.body,
    });
  }
}
