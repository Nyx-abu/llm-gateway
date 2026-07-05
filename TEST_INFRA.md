# Playwright E2E Test Infrastructure

This document outlines the directory layout, setup instructions, execution workflows, and testing patterns for the End-to-End (E2E) test suite of the `llm-gateway` project.

---

## 1. Overview & Test Architecture

The E2E test suite validates the entire gateway stack (Envoy Proxy, Go ext_proc Sidecar, and Mock Upstream Server) as an opaque system. Tests are written in TypeScript using the Playwright API testing framework, targeting the Envoy entrypoint on port `8080`.

```
                    ┌───────────────────────────┐
                    │  Playwright Test Runner   │
                    └─────────────┬─────────────┘
                                  │
                                  │ (HTTP requests / assertions)
                                  ▼
                            [Port 8080]
                    ┌───────────────────────────┐
                    │        Envoy Proxy        │
                    └───────────────────────────┘
```

## 2. Directory Layout

The E2E test suite resides in the `tests/e2e/` directory and configuration files reside in the root:

```
d:\llm-gateway\
├── package.json                # Scripts & DevDependencies for E2E tests
├── tsconfig.json               # TypeScript Compiler configuration
├── playwright.config.ts        # Playwright Test Config (Timeouts, Parallelism, Workers)
├── TEST_INFRA.md               # Infrastructure documentation
├── TEST_READY.md               # Test runner command & coverage checklists
└── tests/
    └── e2e/
        ├── helpers/
        │   └── client.ts       # HTTP Gateway client wrapper
        └── specs/
            ├── tier1_happy_path.spec.ts  # F1: Model Routing happy paths
            ├── tier2_boundary.spec.ts    # F3: Out-of-bounds inputs, edge header conditions
            ├── tier3_cross.spec.ts       # F1+F2+F3: Provider fallback, header/auth preservation
            └── tier4_workloads.spec.ts   # Concurrency, flakiness, delays, connection reuse
```

## 3. Playwright Configuration (`playwright.config.ts`)

The test configuration enforces the following policies:
*   **Execution Target**: Base URL targeting `http://localhost:8080` (or dynamically loaded via `GATEWAY_URL`).
*   **Parallelism**: Fully parallel mode enabled (`fullyParallel: true`).
*   **Workers**: Worker count limited to `4` (or adjusted for system cores) to prevent port conflicts when hitting localhost interfaces concurrently in CI.
*   **Timeouts**: Global test timeout of `30,000ms`, with individual HTTP requests/expects timing out after `5,000ms`.
*   **Retries**: `2` retries in CI environments; `0` retries for local execution.
*   **Reporters**: `list` and `html` reporters enabled.

## 4. Test Execution & CLI Commands

All E2E commands are managed via `npm run` from the project root:

### Package Scripts (`package.json`)
*   `npm run test:e2e`: Runs all specs (Tiers 1 to 4).
*   `npm run test:e2e:tier1`: Runs the Tier 1 Happy Path tests.
*   `npm run test:e2e:tier2`: Runs the Tier 2 Boundary/Edge Case tests.
*   `npm run test:e2e:tier3`: Runs the Tier 3 Cross-Feature tests.
*   `npm run test:e2e:tier4`: Runs the Tier 4 Workloads tests.

### Local Workflow Instructions
1.  **Start Services**:
    ```bash
    docker-compose up -d --build
    ```
2.  **Run All Tests**:
    ```bash
    npm run test:e2e
    ```
3.  **Stop Services**:
    ```bash
    docker-compose down -v
    ```

## 5. Mock Server Control Interfaces

To keep tests decoupled from mock server internals, tests configure mock failures using HTTP headers passed with the client request. This simulates external error states without requiring direct database or control socket interaction:
*   `x-inject-error: true`: Triggers a 500 error on the active provider.
*   `x-inject-openai-status: 500`: Targeted OpenAI upstream failure.
*   `x-inject-anthropic-status: 500`: Targeted Anthropic upstream failure.
*   `x-inject-status: 429`: Triggers a 429 rate limit error on the active provider.
*   `x-inject-openai-delay-ms: 5000` (or `x-inject-delay: 5000`): Injects a 5-second sleep before responding.
*   `x-inject-connection-drop: true`: Force upstream to drop TCP connection immediately.
*   `x-inject-corrupt-body: true`: Force upstream to return malformed/truncated JSON payload.

## 6. CI/CD Integration & Reporting

Tests run on pull requests and main branch builds:
*   **Artifacts**: Playwright reports (`playwright-report/` directory) are archived on failure.
*   **Clean-up**: Hook scripts verify `docker-compose down` runs post-test to clean up container states.
