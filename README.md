<div align="center">

<h1>llm-gateway</h1>

<p><strong>A local LLM API gateway that can choose the right provider, retry on another provider when one fails, and prove the behavior with tests.</strong></p>

<p>
  <img alt="Go 1.22" src="https://img.shields.io/badge/Go-1.22-00ADD8?style=for-the-badge&logo=go&logoColor=white">
  <img alt="Envoy v1.29.1" src="https://img.shields.io/badge/Envoy-v1.29.1-AC6199?style=for-the-badge">
  <img alt="gRPC ext_proc" src="https://img.shields.io/badge/gRPC-ext__proc-244C5A?style=for-the-badge">
  <img alt="Docker Compose" src="https://img.shields.io/badge/Docker_Compose-ready-2496ED?style=for-the-badge&logo=docker&logoColor=white">
  <img alt="Playwright E2E" src="https://img.shields.io/badge/Playwright-E2E-2EAD33?style=for-the-badge&logo=playwright&logoColor=white">
</p>

<p>
  <a href="#about">About</a> |
  <a href="#the-short-version">Short Version</a> |
  <a href="#how-it-works">How It Works</a> |
  <a href="#deep-dive">Deep Dive</a> |
  <a href="#quick-start">Quick Start</a> |
  <a href="#testing">Testing</a> |
  <a href="#tags">Tags</a>
</p>

</div>

---

## About

`llm-gateway` is a small but realistic gateway for LLM traffic.

You send one request to the gateway. The gateway looks at the model you asked for, decides which provider should handle it, sends the request there, and can fall back to the other provider if the first one fails.

In this repo, the "providers" are local mock services. That means you can test routing, failures, retries, and edge cases without real API keys, real bills, or real outages.

## The Short Version

Think of this project as a smart front door for LLM APIs.

| If you are... | Read this as... |
| --- | --- |
| New to gateways | A single API entrypoint that sends each request to the right backend. |
| Building an AI product | A pattern for provider fallback, model routing, and failure testing. |
| An infrastructure engineer | Envoy with a Go `ext_proc` sidecar that mutates headers and uses internal redirects for fallback. |
| Testing reliability | A controlled lab where upstreams can return `500`, delay, drop connections, or send malformed JSON. |

### What problem does it solve?

LLM apps often talk to more than one provider. That creates practical questions:

| Question | What this gateway demonstrates |
| --- | --- |
| Which provider should handle this model? | Route by `x-model`, endpoint path, and fallback state. |
| What happens if the primary provider fails? | Convert `5xx` responses into a controlled fallback request. |
| How do we avoid retry loops? | Mark fallback requests and allow only one internal redirect. |
| How can we test failure behavior locally? | Use a mock upstream with fault-injection headers. |
| How can we prove it works? | Run unit tests and Playwright E2E tests against the full stack. |

## System At A Glance

```mermaid
flowchart LR
  user["App or API Client"] -->|"POST /v1/chat/completions\nor POST /v1/messages"| envoy["Envoy Gateway\nlocalhost:8080"]
  envoy <-->|"Ask: how should\nthis request be handled?"| sidecar["Go ext_proc Sidecar\nlocalhost:50051"]
  envoy -->|"OpenAI-style route"| openai["Mock OpenAI Backend\nlocalhost:8081"]
  envoy -->|"Anthropic-style route"| anthropic["Mock Anthropic Backend\nlocalhost:8081"]
  openai --> envoy
  anthropic --> envoy
  envoy --> user
```

The gateway has three moving parts:

| Part | Plain-English role | Technical role |
| --- | --- | --- |
| Envoy | The traffic router | HTTP proxy, route matcher, `ext_proc` caller, internal redirect executor |
| Go sidecar | The decision helper | gRPC external processor that reads and mutates headers |
| Mock server | The fake provider lab | Local OpenAI/Anthropic simulator with fault injection |

## How It Works

### 1. Normal request flow

```mermaid
sequenceDiagram
  participant Client
  participant Envoy
  participant Sidecar as Go ext_proc sidecar
  participant Provider as Mock provider

  Client->>Envoy: Request with x-model: gpt-4o
  Envoy->>Sidecar: Request headers
  Sidecar-->>Envoy: Set x-llm-provider: openai
  Envoy->>Provider: Forward to OpenAI mock route
  Provider-->>Envoy: 200 response
  Envoy-->>Client: Return provider response
```

The sidecar does not generate LLM answers. It only helps Envoy decide where the request should go.

### 2. Fallback request flow

```mermaid
sequenceDiagram
  participant Client
  participant Envoy
  participant Sidecar as Go ext_proc sidecar
  participant Primary as Primary mock provider
  participant Fallback as Fallback mock provider

  Client->>Envoy: Request with x-model: gpt-4o
  Envoy->>Sidecar: Request headers
  Sidecar-->>Envoy: Route to OpenAI
  Envoy->>Primary: Send request
  Primary-->>Envoy: 500 response
  Envoy->>Sidecar: Response headers
  Sidecar-->>Envoy: Change status to 307 and set fallback location
  Envoy->>Fallback: Internal redirect to Anthropic route
  Fallback-->>Envoy: 200 response
  Envoy-->>Client: Return fallback response
```

From the client's point of view, this still feels like one request. Envoy handles the fallback internally.

### 3. Provider decision table

| Request signal | Provider selected |
| --- | --- |
| `x-model: gpt-4o` | OpenAI mock |
| `x-model: gpt-4-turbo` | OpenAI mock |
| `x-model: o1-mini` | OpenAI mock |
| `x-model: claude-3-5-sonnet` | Anthropic mock |
| `POST /v1/chat/completions` with no known model | OpenAI mock |
| `POST /v1/messages` with no known model | Anthropic mock |
| `/v1/messages?fallback=true` | Anthropic mock |
| `/v1/chat/completions?fallback=true` | OpenAI mock |

### 4. Failure behavior

| Upstream result | Gateway behavior |
| --- | --- |
| `2xx` | Return the response normally. |
| `4xx` | Return the response normally. Client or request problem, not a provider outage. |
| Primary `5xx` | Try the fallback provider through an Envoy internal redirect. |
| Fallback `5xx` | Return the failure. Do not retry forever. |

## Deep Dive

This section is for people who want to understand the internals.

### Why Envoy?

Envoy is doing the heavy traffic work:

- Listening on `localhost:8080`
- Matching public API paths like `/v1/chat/completions` and `/v1/messages`
- Calling the external processing sidecar before routing
- Re-checking routes after the sidecar adds headers
- Rewriting paths to the mock upstream routes
- Performing one internal redirect when fallback is needed

The key point: Envoy stays the data plane. The Go sidecar makes small decisions, but Envoy still owns routing.

### Why a Go `ext_proc` sidecar?

Envoy's External Processing API lets a separate service inspect request and response metadata over gRPC.

In this project, the sidecar watches two phases:

| Phase | What the sidecar reads | What it changes |
| --- | --- | --- |
| Request headers | `:path`, `x-model`, fallback markers | Sets `x-llm-provider`; sometimes normalizes `x-model` |
| Response headers | `:status` | Turns primary `5xx` into `307` plus `location` for fallback |

The sidecar lives in `pkg/extproc/server.go`.

### Why clear Envoy's route cache?

Envoy chooses a route early. But this gateway needs the sidecar to add `x-llm-provider` first.

So after the sidecar mutates request headers, it asks Envoy to clear the route cache. Envoy then evaluates routes again, now with the new provider header available.

Without that step, Envoy could keep using the first route it picked before the sidecar made its decision.

### Why use a `307` for fallback?

The sidecar does not directly call the fallback provider. Instead, it changes a failed primary response into:

```text
:status: 307
location: /v1/messages?fallback=true
```

Envoy sees that `307`, treats it as an internal redirect, and sends the request to the fallback route.

This keeps fallback inside the proxy layer instead of making the sidecar become a second HTTP client.

### How retry loops are avoided

Fallback requests include a marker:

```text
fallback=true
```

The sidecar checks for that marker. If a request is already a fallback request and the fallback provider also fails, the sidecar does not create another redirect.

That keeps one bad upstream from turning into an infinite request loop.

## Quick Start

### Prerequisites

- Docker or Docker Desktop with Compose support
- Node.js 20+ and npm
- Go 1.22+ if you want to run Go unit tests directly on your machine

### Run the local stack

```bash
npm ci
docker compose up --build -d
npm run test:e2e
docker compose down -v
```

If your environment still uses the older Compose binary, use `docker-compose` instead of `docker compose`.

### Try the OpenAI-style route

```bash
curl -i http://localhost:8080/v1/chat/completions \
  -H "content-type: application/json" \
  -H "authorization: Bearer local-test-key" \
  -H "x-model: gpt-4o" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}'
```

### Try the Anthropic-style route

```bash
curl -i http://localhost:8080/v1/messages \
  -H "content-type: application/json" \
  -H "authorization: Bearer local-test-key" \
  -H "x-model: claude-3-5-sonnet" \
  -d '{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"hello"}]}'
```

### Force fallback

```bash
curl -i http://localhost:8080/v1/chat/completions \
  -H "content-type: application/json" \
  -H "authorization: Bearer local-test-key" \
  -H "x-model: gpt-4o" \
  -H "x-inject-openai-status: 500" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"force fallback"}]}'
```

No real OpenAI or Anthropic request is made. Everything runs against local mocks.

## Failure Lab

Use these headers to make the mock providers misbehave on purpose.

| Header | What it does |
| --- | --- |
| `x-inject-error: true` | Forces a generic `500`. |
| `x-inject-openai-status: 500` | Forces the OpenAI mock to return that status. |
| `x-inject-anthropic-status: 503` | Forces the Anthropic mock to return that status. |
| `x-inject-status: 429` | Forces a generic status from the active provider. |
| `x-inject-delay: 5000` | Waits before responding. |
| `x-inject-openai-delay-ms: 5000` | Delays only OpenAI mock handling. |
| `x-inject-anthropic-delay-ms: 5000` | Delays only Anthropic mock handling. |
| `x-inject-connection-drop: true` | Drops the upstream TCP connection. |
| `x-inject-corrupt-body: true` | Returns malformed JSON. |

## Testing

### Go unit tests

```bash
go test ./...
```

These tests cover the mock handlers and the sidecar's routing/fallback decisions.

### Playwright E2E tests

```bash
npm run test:e2e
npm run test:e2e:tier1
npm run test:e2e:tier2
npm run test:e2e:tier3
npm run test:e2e:tier4
```

| Tier | What it checks | Cases |
| --- | --- | --- |
| Tier 1 | Normal model routing | 18 |
| Tier 2 | Missing headers, bad inputs, upstream failures | 18 |
| Tier 3 | Combined routing and fallback scenarios | 5 |
| Tier 4 | Heavier real-world workload patterns | 7 |

By default, Playwright targets `http://localhost:8080`. Override it with:

```bash
GATEWAY_URL=http://localhost:8080 npm run test:e2e
```

## Repository Layout

```text
.
|-- cmd/
|   |-- mock-server/        # Local OpenAI/Anthropic mock HTTP server
|   `-- sidecar/            # gRPC ext_proc sidecar entrypoint
|-- docker/                 # Multi-stage service Dockerfiles
|-- envoy/
|   `-- envoy.yaml          # Envoy routes, clusters, filters, redirects
|-- pkg/
|   `-- extproc/            # External processing server implementation
|-- tests/
|   `-- e2e/                # Playwright API tests and gateway client helper
|-- docker-compose.yaml     # Local three-service stack
|-- go.mod                  # Go module definition
|-- package.json            # Playwright scripts
|-- PROJECT.md              # Architecture and milestone notes
|-- TEST_INFRA.md           # E2E infrastructure details
`-- TEST_READY.md           # Test readiness checklist
```

## Ports

| Port | Service | Purpose |
| --- | --- | --- |
| `8080` | Envoy | Public gateway entrypoint |
| `8001` | Envoy admin | Envoy diagnostics and admin API |
| `50051` | Go sidecar | gRPC `ext_proc` service |
| `8081` | Mock server | Local OpenAI/Anthropic-style upstreams |

## Current Status

| Milestone | Status |
| --- | --- |
| E2E testing track | Done |
| Mock upstream server | Done |
| Go `ext_proc` sidecar | Done |
| Envoy routing configuration | Done |
| Local Docker integration | Done |
| End-to-end validation | In progress |
| Adversarial hardening | Planned |

## Notes And Limits

- This is a local test gateway, not a production billing/auth system.
- The mock server does not validate real API keys.
- The fallback behavior is intentionally scoped to `5xx` provider failures.
- `4xx` responses generally pass through because they usually mean the request needs fixing.
- Docker must be running before the full E2E path can be tested.

## Tags

Recommended GitHub repository topics:

`envoy` `go` `grpc` `ext-proc` `llm` `llm-gateway` `api-gateway` `model-routing` `fallback` `playwright` `docker-compose` `openai` `anthropic`

## License

No license file is currently included. Add a license before distributing, packaging, or accepting external reuse.
