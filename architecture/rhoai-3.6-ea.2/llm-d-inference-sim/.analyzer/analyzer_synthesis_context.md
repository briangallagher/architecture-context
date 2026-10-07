# Analyzer Synthesis Context: llm-d-inference-sim

This file is a bounded, source-linked projection. Read it before the full analyzer JSON. It does not replace the authoritative JSON.

## Coverage Findings

- **crds (not-verified)**: 0 crds facts extracted; absence is not proven by the available coverage
- **grpc_services (confirmed-empty)**: 0 grpc_services facts extracted
- **http_endpoints (observed)**: 24 http_endpoints facts extracted [source: pkg/communication/http.go:71, pkg/communication/http.go:72, pkg/communication/http.go:73, pkg/communication/http.go:74, pkg/communication/http.go:75, pkg/communication/http.go:76, pkg/communication/http.go:77, pkg/communication/http.go:78, pkg/communication/http.go:80, pkg/communication/http.go:83, pkg/communication/http.go:85, pkg/communication/http.go:87, pkg/communication/http.go:88, pkg/communication/http.go:89, pkg/communication/http.go:90, pkg/communication/http.go:91, pkg/engine/vllm/transport.go:30, pkg/engine/vllm/transport.go:31, pkg/engine/vllm/transport.go:32, pkg/engine/vllm/transport.go:33, pkg/engine/vllm/transport.go:35, pkg/engine/vllm/transport.go:36, pkg/engine/vllm/transport.go:37, pkg/engine/vllm/transport.go:38]
- **services (observed)**: 1 services facts extracted [source: deploy/deployments.yaml:80]
- **ingress (confirmed-empty)**: 0 ingress facts extracted
- **webhooks (not-verified)**: 0 webhooks facts extracted; absence is not proven by the available coverage

## Deterministic Cross-References


## Behavioral Evidence

No bounded behavioral evidence was extracted.

## Gap Evidence Index

### configuration_lifecycle

- **Question:** What lifecycle, command, probes, and deployment configuration surround this entrypoint?
  **Expected signal:** main command, startup path, probe, signal handling, or workload mapping
  **Candidate:** `Dockerfile`:33 (Dockerfile:ENTRYPOINT)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** What lifecycle, command, probes, and deployment configuration surround this entrypoint?
  **Expected signal:** main command, startup path, probe, signal handling, or workload mapping
  **Candidate:** `Dockerfile.zmq`:19 (Dockerfile.zmq:CMD)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** What lifecycle, command, probes, and deployment configuration surround this entrypoint?
  **Expected signal:** main command, startup path, probe, signal handling, or workload mapping
  **Candidate:** `cmd/dataset-tool/main.go`:29 (dataset-tool)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** What lifecycle, command, probes, and deployment configuration surround this entrypoint?
  **Expected signal:** main command, startup path, probe, signal handling, or workload mapping
  **Candidate:** `cmd/llm-d-inference-sim/main.go`:47 (llm-d-inference-sim)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
### http_endpoints

- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `pkg/communication/http.go`:75 (/v1/chat/completions/derender, POST, pkg/communication)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `pkg/communication/http.go`:85 (/metrics, GET, pkg/communication)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `pkg/communication/http.go`:87 (/health, GET, pkg/communication)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `pkg/communication/http.go`:88 (/health/ready, GET, pkg/communication)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `pkg/communication/http.go`:89 (/tokenize, POST, pkg/communication)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `pkg/communication/http.go`:90 (/admin/config, GET, pkg/communication)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `pkg/communication/http.go`:91 (/admin/config, POST, pkg/communication)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `pkg/engine/vllm/transport.go`:30 (/inference/v1/generate, POST, pkg/engine/vllm)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `pkg/engine/vllm/transport.go`:33 (/fake_metrics, POST, pkg/engine/vllm)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `pkg/engine/vllm/transport.go`:35 (/query, GET, pkg/engine/vllm)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `pkg/engine/vllm/transport.go`:36 (/sleep, POST, pkg/engine/vllm)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Does this endpoint have additional dynamic routes or a concrete handler/owner?
  **Expected signal:** route registration, handler binding, middleware, or owner symbol
  **Candidate:** `pkg/engine/vllm/transport.go`:38 (/is_sleeping, GET, pkg/engine/vllm)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
### internal_dependencies

- **Question:** Where is this internal dependency invoked and what is the interaction boundary?
  **Expected signal:** import, client call, queue, or controller handoff
  **Candidate:** `pkg/kvcache/kv_cache.go`:29 (Go library, llm-d-router)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
### services

- **Question:** Which container listener, probe, and service mapping expose this workload?
  **Expected signal:** container port, probe, service account, or lifecycle configuration
  **Candidate:** `deploy/deployments.yaml`:1 (vllm-sim)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship
- **Question:** Which workload owns this Service and does its target port match a runtime listener?
  **Expected signal:** selector, target deployment, port mapping, or listener
  **Candidate:** `deploy/deployments.yaml`:80 (vllm-sim)
  **Status:** candidate; **Limitations:** candidate location only; source inspection is required to establish the relationship

## Section Evidence

### http_endpoints

- GET /admin/config on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:90]
- GET /health on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:87]
- GET /health/ready on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:88]
- GET /is_sleeping on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/engine/vllm [source: pkg/engine/vllm/transport.go:38]
- GET /metrics on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:85]
- GET /query on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/engine/vllm [source: pkg/engine/vllm/transport.go:35]
- GET /v1/models on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:83]
- POST /admin/config on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:91]
- POST /fake_metrics on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/engine/vllm [source: pkg/engine/vllm/transport.go:33]
- POST /inference/v1/generate on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/engine/vllm [source: pkg/engine/vllm/transport.go:30]
- POST /sleep on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/engine/vllm [source: pkg/engine/vllm/transport.go:36]
- POST /tokenize on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:89]
- POST /v1/chat/completions on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:71]
- POST /v1/chat/completions/derender on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:75]
- POST /v1/chat/completions/render on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:73]
- POST /v1/completions on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:72]
- POST /v1/completions/derender on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:76]
- POST /v1/completions/render on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:74]
- POST /v1/embeddings on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:80]
- POST /v1/load_lora_adapter on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/engine/vllm [source: pkg/engine/vllm/transport.go:31]
- POST /v1/messages on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:78]
- POST /v1/responses on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/communication [source: pkg/communication/http.go:77]
- POST /v1/unload_lora_adapter on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/engine/vllm [source: pkg/engine/vllm/transport.go:32]
- POST /wake_up on port ; transport=HTTP/1.1 encryption= auth= owner=pkg/engine/vllm [source: pkg/engine/vllm/transport.go:37]
### internal_dependencies

- llm-d-router interaction=Go library role=runtime-library purpose=Use runtime packages from github.com/llm-d/llm-d-router [source: pkg/kvcache/kv_cache.go:29]
### services

- vllm-sim port=8000 target=8000 protocol=TCP encryption= auth= [source: deploy/deployments.yaml:80]

## Cross-Cutting Evidence

### deployment_topology

- **observed**: Deployment workload vllm-sim uses service account  and 1 container(s) [source: deploy/deployments.yaml:1]
- **observed**: Service vllm-sim targets vllm-sim with 1 port(s) [source: deploy/deployments.yaml:80]
### disconnected_deployment

- **unresolved**: No complete deterministic evidence family was extracted; targeted source/configuration review may be required [source: coverage:disconnected_deployment]
### high_availability

- **unresolved**: No complete deterministic evidence family was extracted; targeted source/configuration review may be required [source: coverage:high_availability]
### ingress

- **observed**: HTTP GET /admin/config is owned by pkg/communication [source: pkg/communication/http.go:90]
- **observed**: HTTP GET /health is owned by pkg/communication [source: pkg/communication/http.go:87]
- **observed**: HTTP GET /health/ready is owned by pkg/communication [source: pkg/communication/http.go:88]
- **observed**: HTTP GET /is_sleeping is owned by pkg/engine/vllm [source: pkg/engine/vllm/transport.go:38]
- **observed**: HTTP GET /metrics is owned by pkg/communication [source: pkg/communication/http.go:85]
- **observed**: HTTP GET /query is owned by pkg/engine/vllm [source: pkg/engine/vllm/transport.go:35]
- **observed**: HTTP GET /v1/models is owned by pkg/communication [source: pkg/communication/http.go:83]
- **observed**: HTTP POST /admin/config is owned by pkg/communication [source: pkg/communication/http.go:91]
- **observed**: HTTP POST /fake_metrics is owned by pkg/engine/vllm [source: pkg/engine/vllm/transport.go:33]
- **observed**: HTTP POST /inference/v1/generate is owned by pkg/engine/vllm [source: pkg/engine/vllm/transport.go:30]
- **observed**: HTTP POST /sleep is owned by pkg/engine/vllm [source: pkg/engine/vllm/transport.go:36]
- **observed**: HTTP POST /tokenize is owned by pkg/communication [source: pkg/communication/http.go:89]
- **observed**: HTTP POST /v1/chat/completions is owned by pkg/communication [source: pkg/communication/http.go:71]
- **observed**: HTTP POST /v1/chat/completions/derender is owned by pkg/communication [source: pkg/communication/http.go:75]
- **observed**: HTTP POST /v1/chat/completions/render is owned by pkg/communication [source: pkg/communication/http.go:73]
- **observed**: HTTP POST /v1/completions is owned by pkg/communication [source: pkg/communication/http.go:72]
- **observed**: HTTP POST /v1/completions/derender is owned by pkg/communication [source: pkg/communication/http.go:76]
- **observed**: HTTP POST /v1/completions/render is owned by pkg/communication [source: pkg/communication/http.go:74]
- **observed**: HTTP POST /v1/embeddings is owned by pkg/communication [source: pkg/communication/http.go:80]
- **observed**: HTTP POST /v1/load_lora_adapter is owned by pkg/engine/vllm [source: pkg/engine/vllm/transport.go:31]
- **observed**: HTTP POST /v1/messages is owned by pkg/communication [source: pkg/communication/http.go:78]
- **observed**: HTTP POST /v1/responses is owned by pkg/communication [source: pkg/communication/http.go:77]
- **observed**: HTTP POST /v1/unload_lora_adapter is owned by pkg/engine/vllm [source: pkg/engine/vllm/transport.go:32]
- **observed**: HTTP POST /wake_up is owned by pkg/engine/vllm [source: pkg/engine/vllm/transport.go:37]
### security

- **dependency-signal**: tls-config targets crypto/tls: TLS configuration import [source: pkg/common/certs.go, pkg/communication/http_server_tls.go]
### supply_chain

- **unresolved**: No complete deterministic evidence family was extracted; targeted source/configuration review may be required [source: coverage:supply_chain]
