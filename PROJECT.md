# Project: llm-gateway

## Architecture
The `llm-gateway` consists of three core components:
1. **Envoy Proxy**: The primary entry point for client traffic. It handles TLS termination, request routing, retry/fallback mechanisms, and integrates with the Go `ext_proc` sidecar.
2. **Go ext_proc Sidecar**: A gRPC server implementing the Envoy External Processing (`ext_proc`) API. It inspects incoming client request headers to determine the model, updates routing headers, and intercepts response headers to trigger transparent fallback to a secondary provider if the primary provider fails.
3. **Mock Upstream Server**: A Go HTTP server that simulates responses from both OpenAI and Anthropic API endpoints. It allows injecting artificial failures (e.g., 500 status codes) to test the gateway's fallback logic.

```
       Client
         │
         ▼
 ┌───────────────┐
 │  Envoy Proxy  │ ◄──────► Go ext_proc Sidecar (gRPC 50051)
 └───────┬───────┘
         │
         ├───► OpenAI Upstream (Mock HTTP 8081)
         │
         └───► Anthropic Upstream (Mock HTTP 8081)
```

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|---|---|---|---|
| M1 | E2E Testing Track | Design and implement Playwright-based test suite, verification runner, and CI integration | None | DONE (Conv ID: 4a6a060d-93fc-466e-b6f8-0f94df9f2318) |
| M2 | Mock Upstream Server | Implement Go HTTP mock server with error injection capabilities | None | DONE (Conv ID: acfd5bb7-d724-4b2b-a8d3-5f0949b98139) |
| M3 | Go ext_proc Sidecar | Implement Go gRPC ext_proc server with request/response headers modification logic | M2 | DONE (Conv ID: 84d0163b-262f-4343-afef-54539b0669d3) |
| M4 | Envoy Configuration | Configure Envoy proxy to route traffic, invoke ext_proc, and handle internal redirects for fallback | M3 | DONE (Conv ID: 41da9d6c-20d0-423c-86bf-98472918acef) |
| M5 | Local Docker Integration | Package all services in Docker Compose for local execution | M2, M3, M4 | DONE (Conv ID: 41da9d6c-20d0-423c-86bf-98472918acef) |
| M6 | End-to-End Validation | Run E2E test suite against the full Docker Compose stack, verify happy-path and fallback | M1, M5 | IN_PROGRESS (Conv ID: e9c1c393-cfca-4c61-9a2f-ca7fe4be9837) |
| M7 | Adversarial Hardening | Implement adversarial testing to uncover edge cases and verify recovery robustness | M6 | PLANNED |

## Interface Contracts
### Client ↔ Envoy Proxy
- Client sends HTTP POST to `http://localhost:8080/v1/chat/completions` (OpenAI format) or `http://localhost:8080/v1/messages` (Anthropic format).
- Headers:
  - `x-model`: Specifies target model (e.g. `gpt-4o`, `claude-3-5-sonnet`).
  - `authorization`: Bearer token (simulated).

### Envoy Proxy ↔ Go ext_proc Sidecar
- Protocol: gRPC over TCP (port 50051).
- Envoy invokes `ext_proc` on Request Headers and Response Headers.
- Request Headers processing:
  - Sidecar inspects `x-model`.
  - Sidecar sets `x-llm-provider` routing header to `openai` or `anthropic`.
  - Envoy routes the request to the corresponding cluster based on `x-llm-provider`.
- Response Headers processing:
  - If upstream returns 5xx error, sidecar intercepts it.
  - Sidecar returns an `ImmediateResponse` redirecting the client internally, or returns headers modifying the response to trigger an internal redirect.
  - Specifically, sidecar modifies status to `307` and sets `location` header to `/fallback/...` to trigger Envoy's `internal_redirect_policy`.

### Envoy Proxy ↔ Mock Upstream Server
- Mock server listens on port 8081.
- Endpoint `/openai/v1/chat/completions` maps to the OpenAI mock cluster.
- Endpoint `/anthropic/v1/messages` maps to the Anthropic mock cluster.
- Failure injection is controlled via query params or request headers (e.g. `x-inject-error: true`).

## Code Layout
- `cmd/mock-server/main.go`: Mock Upstream Server entrypoint.
- `cmd/sidecar/main.go`: Go ext_proc sidecar entrypoint.
- `pkg/extproc/server.go`: ext_proc gRPC implementation.
- `envoy/envoy.yaml`: Envoy configuration file.
- `docker-compose.yaml`: Docker Compose file packaging all services.
- `tests/e2e/`: Playwright E2E tests.
- `package.json`: NPM package config for E2E tests.
