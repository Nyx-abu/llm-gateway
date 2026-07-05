# Project Plan: llm-gateway

This document outlines the execution plan for building the `llm-gateway` with Envoy and a Go ext_proc sidecar.

## Scope and Objectives
We are building a gateway that uses Envoy as a data plane and a Go gRPC external processing (`ext_proc`) sidecar to inspect request headers, determine the target model, configure routing, and intercept upstream errors to transparently fall back from a primary provider (e.g. OpenAI) to a fallback provider (e.g. Anthropic).

For a detailed view of the architecture and interface contracts, refer to [PROJECT.md](./PROJECT.md).

## Milestone Plan

### Milestone 1: E2E Testing Track (Playwright)
- Design and build the E2E testing framework in `tests/e2e/`.
- Enumerate test cases across 4 tiers (Feature Coverage, Boundary/Corner Cases, Cross-Feature Combinations, and Real-World Scenarios).
- Establish test infrastructure and runner command.
- Generate `TEST_INFRA.md` and `TEST_READY.md`.

### Milestone 2: Mock Upstream Server
- Build a Go HTTP mock server in `cmd/mock-server/`.
- Support mock responses for OpenAI and Anthropic formats.
- Support error injection via request headers or path parameters to simulate 500 errors.
- Verify using unit tests.

### Milestone 3: Go ext_proc Sidecar
- Build a Go gRPC server implementing Envoy's `ext_proc` proto interface.
- Implement header parser for request model routing.
- Implement response interception logic for status codes to change response headers to a redirect for fallback routing.
- Verify with unit tests.

### Milestone 4: Envoy Configuration
- Draft Envoy configuration (`envoy/envoy.yaml`) defining routes, clusters, ext_proc filter, and internal redirect policy.
- Ensure proper mapping of paths to mock upstream server clusters.

### Milestone 5: Local Docker Integration
- Create a `docker-compose.yaml` to spin up Envoy, the Go ext_proc sidecar, and the mock upstream server.
- Ensure all services start up and can communicate correctly.

### Milestone 6: End-to-End Validation
- Run the Playwright test suite against the live Docker Compose stack.
- Verify both happy-path routing and transparent fallback behavior.

### Milestone 7: Adversarial Hardening
- Conduct adversarial coverage audits and generate stress-test workloads.
- Fix any bugs or edge cases found.

## Execution Model
- Work will be conducted using specialized agents (`teamwork_preview_explorer`, `teamwork_preview_worker`, `teamwork_preview_reviewer`, `teamwork_preview_challenger`, and `teamwork_preview_auditor`).
- We will strictly follow the Project pattern: first establishing E2E test infra, then building the implementation, verifying at each milestone, and running final validation.
