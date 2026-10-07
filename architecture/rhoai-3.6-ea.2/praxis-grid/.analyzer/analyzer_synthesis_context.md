# Analyzer Synthesis Context: grid

This file is a bounded, source-linked projection. Read it before the full analyzer JSON. It does not replace the authoritative JSON.

## Coverage Findings

- **crds (observed)**: 3 crds facts extracted [source: deploy/crds/gridnetwork.yaml:1, deploy/crds/gridsite.yaml:1, deploy/crds/inferenceprovider.yaml:1]
- **grpc_services (not-verified)**: 0 grpc_services facts extracted; absence is not proven by the available coverage
- **http_endpoints (observed)**: 13 http_endpoints facts extracted [source: mock-providers/src/anthropic.rs:25, mock-providers/src/anthropic.rs:26, mock-providers/src/bedrock.rs:26, mock-providers/src/bedrock.rs:27, mock-providers/src/openai.rs:37, mock-providers/src/openai.rs:38, mock-providers/src/openai.rs:39, mock-providers/src/openai.rs:40, mock-providers/src/vertex.rs:31, operator/src/main.rs:606, operator/src/main.rs:607, overlay-sync/src/main.rs:265, overlay-sync/src/main.rs:267]
- **services (observed)**: 2 services facts extracted
- **ingress (confirmed-empty)**: 0 ingress facts extracted
- **webhooks (confirmed-empty)**: 0 webhooks facts extracted

## Deterministic Cross-References


## Behavioral Evidence

No bounded behavioral evidence was extracted.

## Gap Evidence Index

### authentication

- **Question:** Where is authentication enforced for this surface, and is it conditional?
  **Expected signal:** middleware, filter, policy, or enforcement branch
  **Candidate:** `deploy/operator/deployment.yaml`:1 (:9090/healthz, None)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Where is authentication enforced for this surface, and is it conditional?
  **Expected signal:** middleware, filter, policy, or enforcement branch
  **Candidate:** `deploy/operator/deployment.yaml`:1 (:9090/readyz, None)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Where is authentication enforced for this surface, and is it conditional?
  **Expected signal:** middleware, filter, policy, or enforcement branch
  **Candidate:** `mock-providers/src/anthropic.rs`:26 (/api/v1/*, /api/v2/*, Header passthrough)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Where is authentication enforced for this surface, and is it conditional?
  **Expected signal:** middleware, filter, policy, or enforcement branch
  **Candidate:** `mock-providers/src/anthropic.rs`:26 (/health, /info, None)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
### authorization

- **Question:** Which workload identity receives this role and where is it used?
  **Expected signal:** service account or subject-to-workload binding
  **Candidate:** `deploy/operator/cluster-role-binding-crd.yaml`:1 (grid-operator-crd)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Which controller, handler, or service account exercises this RBAC policy?
  **Expected signal:** role rules, binding subject, handler, or controller identity
  **Candidate:** `deploy/operator/cluster-role-crd.yaml`:1 (grid-operator-crd)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Which controller, handler, or service account exercises this RBAC policy?
  **Expected signal:** role rules, binding subject, handler, or controller identity
  **Candidate:** `deploy/operator/cluster-role-resources.yaml`:1 (grid-operator-resources)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Which workload identity receives this role and where is it used?
  **Expected signal:** service account or subject-to-workload binding
  **Candidate:** `deploy/operator/role-binding-grid-system.yaml`:1 (grid-operator-resources)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
### http_endpoints

- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `mock-providers/src/anthropic.rs`:25 (/v1/messages, POST)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `mock-providers/src/anthropic.rs`:26 (/health, GET)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `mock-providers/src/bedrock.rs`:26 (/model/{model_id}/converse, POST)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `mock-providers/src/bedrock.rs`:27 (/model/{model_id}/converse-stream, POST)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `mock-providers/src/openai.rs`:37 (/v1/chat/completions, POST)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `mock-providers/src/openai.rs`:39 (/v1/models, GET)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `mock-providers/src/openai.rs`:40 (/metrics, GET)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `mock-providers/src/vertex.rs`:31 (/v1/projects/{project}/locations/{location}/publishers/google/models/{*rest}, POST)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `operator/src/main.rs`:606 (/healthz, GET)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `operator/src/main.rs`:607 (/readyz, GET)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `overlay-sync/src/main.rs`:265 (/livez, GET)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `overlay-sync/src/main.rs`:267 (/status, GET)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
### services

- **Question:** Which container listener, probe, and service mapping expose this workload?
  **Expected signal:** container port, probe, service account, or lifecycle configuration
  **Candidate:** `deploy/operator/deployment.yaml`:1 (grid-operator)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship

## Section Evidence

### authentication

- /api/v1/*, /api/v2/* methods=POST mechanism=Header passthrough enforcement=Application-level filtering policy=Configured headers are forwarded to downstream services [source: mock-providers/src/anthropic.rs:26]
- /health, /info methods=GET mechanism=None enforcement=None policy=Unauthenticated health server [source: mock-providers/src/anthropic.rs:26]
- :9090/healthz methods=GET mechanism=None enforcement=N/A policy=Unauthenticated Kubernetes liveness probe endpoint [source: deploy/operator/deployment.yaml:1]
- :9090/readyz methods=GET mechanism=None enforcement=N/A policy=Unauthenticated Kubernetes readiness probe endpoint [source: deploy/operator/deployment.yaml:1]
### http_endpoints

- GET /health on port ; transport= encryption=TLS 1.2+ (optional) auth=Passthrough headers owner= [source: mock-providers/src/anthropic.rs:26]
- GET /healthz on port ; transport= encryption=TLS 1.2+ (optional) auth=Passthrough headers owner= [source: operator/src/main.rs:606]
- GET /livez on port ; transport= encryption=None auth=None owner= [source: overlay-sync/src/main.rs:265]
- GET /metrics on port ; transport= encryption=TLS 1.2+ (optional) auth=Passthrough headers owner= [source: mock-providers/src/openai.rs:40]
- GET /readyz on port ; transport= encryption=TLS 1.2+ (optional) auth=Passthrough headers owner= [source: operator/src/main.rs:607]
- GET /status on port ; transport= encryption=None auth=None owner= [source: overlay-sync/src/main.rs:267]
- GET /v1/models on port ; transport= encryption=TLS 1.2+ (optional) auth=Passthrough headers owner= [source: mock-providers/src/openai.rs:39]
- POST /model/{model_id}/converse on port ; transport= encryption=TLS 1.2+ (optional) auth=Passthrough headers owner= [source: mock-providers/src/bedrock.rs:26]
- POST /model/{model_id}/converse-stream on port ; transport= encryption=TLS 1.2+ (optional) auth=Passthrough headers owner= [source: mock-providers/src/bedrock.rs:27]
- POST /v1/chat/completions on port ; transport= encryption=TLS 1.2+ (optional) auth=Passthrough headers owner= [source: mock-providers/src/openai.rs:37]
- POST /v1/messages on port ; transport= encryption=TLS 1.2+ (optional) auth=Passthrough headers owner= [source: mock-providers/src/anthropic.rs:25]
- POST /v1/projects/{project}/locations/{location}/publishers/google/models/{*rest} on port ; transport= encryption=TLS 1.2+ (optional) auth=Passthrough headers owner= [source: mock-providers/src/vertex.rs:31]
- POST /v1/responses on port ; transport= encryption=TLS 1.2+ (optional) auth=Passthrough headers owner= [source: mock-providers/src/openai.rs:38]

## Cross-Cutting Evidence

### deployment_topology

- **observed**: Deployment workload grid-operator uses service account grid-operator and 1 container(s) [source: deploy/operator/deployment.yaml:1]
### disconnected_deployment

- **unresolved**: No complete deterministic evidence family was extracted; targeted source/configuration review may be required [source: coverage:disconnected_deployment]
### high_availability

- **unresolved**: No complete deterministic evidence family was extracted; targeted source/configuration review may be required [source: coverage:high_availability]
### ingress

- **unresolved**: No complete deterministic evidence family was extracted; targeted source/configuration review may be required [source: coverage:ingress]
### security

- **observed**: GET /health, /info uses None at None; policy=Unauthenticated health server [source: mock-providers/src/anthropic.rs:26]
- **observed**: GET :9090/healthz uses None at N/A; policy=Unauthenticated Kubernetes liveness probe endpoint [source: deploy/operator/deployment.yaml:1]
- **observed**: GET :9090/readyz uses None at N/A; policy=Unauthenticated Kubernetes readiness probe endpoint [source: deploy/operator/deployment.yaml:1]
- **observed**: POST /api/v1/*, /api/v2/* uses Header passthrough at Application-level filtering; policy=Configured headers are forwarded to downstream services [source: mock-providers/src/anthropic.rs:26]
- **observed**: RBAC role grid-operator-crd grants 3 rule(s) [source: deploy/operator/cluster-role-crd.yaml:1]
- **observed**: RBAC role grid-operator-resources grants 4 rule(s) [source: deploy/operator/cluster-role-resources.yaml:1]
- **dependency-signal**: crypto-library targets hyper-rustls: Rust TLS dependency is present; the cryptographic provider and FIPS mode require configuration or lockfile verification [source: Cargo.toml:33]
- **dependency-signal**: crypto-library targets rustls: Rust TLS dependency is present; the cryptographic provider and FIPS mode require configuration or lockfile verification [source: Cargo.toml:33]
- **dependency-signal**: crypto-library targets tokio-rustls: Rust TLS dependency is present; the cryptographic provider and FIPS mode require configuration or lockfile verification [source: Cargo.toml:60]
- **dependency-signal**: crypto-provider targets ring: Cargo.lock selects ring as a cryptographic provider; ring is not a FIPS-validated provider [source: Cargo.lock:2339]
- **not-extracted**: fips-posture targets FIPS validation: FIPS validation and runtime provider selection are not fully determined by static dependency/build signals [source: Cargo.toml:33]
### supply_chain

- **unresolved**: No complete deterministic evidence family was extracted; targeted source/configuration review may be required [source: coverage:supply_chain]
