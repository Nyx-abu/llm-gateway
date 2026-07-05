# E2E Test Readiness & Coverage Checklists

This document details the test execution command, expected exit codes, and lists the E2E verification coverage checklists across all 4 tiers of tests.

---

## 1. Test Execution Commands & Exit Codes

### Commands
- Run all test tiers: `npm run test:e2e`
- Run Tier 1 only: `npm run test:e2e:tier1`
- Run Tier 2 only: `npm run test:e2e:tier2`
- Run Tier 3 only: `npm run test:e2e:tier3`
- Run Tier 4 only: `npm run test:e2e:tier4`

### Expected Exit Codes
- **`0`**: Success. All tests ran and met their assertions.
- **`1`**: Test failure. One or more assertions failed or a connection/compilation error occurred.
- **`130`**: Execution cancelled/interrupted (e.g. CTRL+C).

---

## 2. E2E Test Coverage Checklists

### Tier 1: Happy Path Model Routing (18 Cases)
- [ ] **TC-T1-01**: Route `gpt-4o` to OpenAI Mock Upstream
- [ ] **TC-T1-02**: Route `gpt-4-turbo` to OpenAI Mock Upstream
- [ ] **TC-T1-03**: Route `gpt-4` to OpenAI Mock Upstream
- [ ] **TC-T1-04**: Route `gpt-3.5-turbo` to OpenAI Mock Upstream
- [ ] **TC-T1-05**: Route `o1-mini` to OpenAI Mock Upstream
- [ ] **TC-T1-06**: Route `o1-preview` to OpenAI Mock Upstream
- [ ] **TC-T1-07**: Case Insensitivity routing check (`GPT-4O`)
- [ ] **TC-T1-08**: Default OpenAI routing when `x-model` header is absent
- [ ] **TC-T1-09**: Route `claude-3-5-sonnet` to Anthropic Mock Upstream
- [ ] **TC-T1-10**: Route `claude-3-opus` to Anthropic Mock Upstream
- [ ] **TC-T1-11**: Route `claude-3-haiku` to Anthropic Mock Upstream
- [ ] **TC-T1-12**: Route `claude-2.1` to Anthropic Mock Upstream
- [ ] **TC-T1-13**: Route `claude-2.0` to Anthropic Mock Upstream
- [ ] **TC-T1-14**: Case Insensitivity routing check (`CLAUDE-3-5-SONNET`)
- [ ] **TC-T1-15**: Default Anthropic routing when `x-model` header is absent
- [ ] **TC-T1-16**: Authentication forwarding to OpenAI
- [ ] **TC-T1-17**: Authentication forwarding to Anthropic
- [ ] **TC-T1-18**: Unrecognized Model Defaulting

### Tier 2: Boundary and Corner Cases (18 Cases)
- [ ] **TC-T2-01**: Missing `x-model` Header
- [ ] **TC-T2-02**: Empty `x-model` Header Value
- [ ] **TC-T2-03**: Unknown or Unsupported Model
- [ ] **TC-T2-04**: Malformed Request JSON Body
- [ ] **TC-T2-05**: Missing Authorization Header
- [ ] **TC-T2-06**: Extremely Large Header Value
- [ ] **TC-T2-07**: Excessive Number of Headers
- [ ] **TC-T2-08**: Duplicate `x-model` Headers
- [ ] **TC-T2-09**: Primary returns 500 Internal Server Error
- [ ] **TC-T2-10**: Primary returns 502 Bad Gateway
- [ ] **TC-T2-11**: Primary returns 503 Service Unavailable
- [ ] **TC-T2-12**: Primary returns 504 Gateway Timeout
- [ ] **TC-T2-13**: Both Primary and Fallback Upstreams Fail (Double Failure)
- [ ] **TC-T2-14**: Upstream Returns 429 Too Many Requests (No Fallback)
- [ ] **TC-T2-15**: Primary Upstream Timeout (Envoy Local Timeout)
- [ ] **TC-T2-16**: Primary Upstream Connection Reset / TCP Drop
- [ ] **TC-T2-17**: Fallback Upstream Times Out
- [ ] **TC-T2-18**: Corrupted Response Body from Primary Upstream

### Tier 3: Cross-Feature Combination Cases (5 Cases)
- [ ] **Case 3.1**: OpenAI-to-Anthropic Fallback with Path and Header-based Routing (F1 + F2 + F3)
- [ ] **Case 3.2**: Anthropic-to-OpenAI Fallback under Error Injection (F1 + F2 + F3)
- [ ] **Case 3.3**: Cascading Failure / Exhausted Fallback (F1 + F2 + F3)
- [ ] **Case 3.4**: Authorization Header Preservation across Fallback (F1 + F2 + F3)
- [ ] **Case 3.5**: Upstream Rate Limit (429) Triggered Fallback (F1 + F2 + F3)

### Tier 4: Real-world Workloads (7 Workloads)
- [ ] **Workload 4.1**: High-Concurrency Chat Stream and Fallback Storm
- [ ] **Workload 4.2**: Flaky Network Simulation (Intermittent 500s and Recovery)
- [ ] **Workload 4.3**: Mixed-Traffic Model Routing and Load Balancing
- [ ] **Workload 4.4**: Large Payload Handling (Prompt and System Instruction Stress Test)
- [ ] **Workload 4.5**: Long-Running Persistent Connection Keep-Alive
- [ ] **Workload 4.6**: Slow Response / Read Timeout Fallback (Upstream Latency)
- [ ] **Workload 4.7**: Dynamic Quota Refusal and Rate-Limit Validation
