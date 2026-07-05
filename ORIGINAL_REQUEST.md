# Original User Request

## Initial Request — 2026-07-05T21:32:43+05:30

# Teamwork Project Prompt — Draft

> Status: Launched
> Goal: Craft prompt → get user approval → delegate to teamwork_preview

Build the `llm-gateway` utilizing an Envoy proxy data plane and a Go gRPC `ext_proc` sidecar for intelligent fallback routing and CRDT quota management.

Working directory: ~/teamwork_projects/llm_gateway
Integrity mode: benchmark

Reference Material: See the `implementation_plan.md` artifact in the workspace for detailed architectural decisions (Envoy + Go Ext-Proc Hybrid).

## Requirements

### R1. Implement the Ext-Proc Go Sidecar
Build a Go gRPC server that implements the Envoy `ext_proc` interface. It should inspect request headers to determine the target model and manipulate Envoy routing headers to enable fallback (e.g., from OpenAI to Anthropic) when upstream errors occur. 

### R2. Configure the Envoy Data Plane
Create an Envoy configuration that acts as the primary reverse proxy. It must route traffic to upstream LLM providers, enforce byte-heuristic rate limiting, and correctly integrate with the Go `ext_proc` sidecar for dynamic routing logic.

### R3. Create a Mock Upstream Testing Server
Implement a simple mock upstream server that simulates LLM provider responses. It should support configurable error injection (e.g., forcing 500 Internal Server Errors) to properly test the fallback routing logic.

### R4. Provide a Local Development Stack
Package the Envoy proxy, Go sidecar, and mock upstream server using Docker Compose so that the entire architecture can be spun up and tested locally with a single command.

### R5. End-to-End Testing with Playwright
Implement an end-to-end test suite using Playwright. The tests should spin up the full gateway stack and programmatically verify that the gateway routes traffic correctly and handles upstream failures by transparently falling back.

## Acceptance Criteria

### Fallback Routing and Integration
- [ ] `docker-compose up` successfully builds and starts Envoy, the Go sidecar, and the mock upstream server without errors.
- [ ] A test script or `curl` command against the Envoy proxy successfully receives a simulated response from the primary mock provider.
- [ ] When the primary mock provider is configured to fail (e.g., 500 error), a request to the Envoy proxy is transparently routed to the fallback provider and returns a successful response.
- [ ] The Go sidecar codebase includes unit tests verifying the fallback decision logic.

### Playwright Testing
- [ ] The Playwright test suite can be run with a single command (e.g., `npm run test:e2e`).
- [ ] Playwright tests successfully assert that the fallback routing behaves correctly under simulated upstream failure conditions.
