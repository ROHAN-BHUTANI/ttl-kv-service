# ttl-kv-service

A high-performance in-memory key-value service written in Go with per-key TTL, lazy and periodic background expiration, HTTP/REST API, graceful shutdown lifecycle, Docker multi-stage distroless packaging, and Kubernetes deployment manifests.

---

## 1. Architecture Overview

```mermaid
flowchart TD
    Client([HTTP Client]) -->|HTTP Port 80| K8sSvc[Kubernetes Service: ttl-kv]

    subgraph Cluster [Kubernetes Cluster / Local Deployment]
        K8sSvc -->|Load Balance| Pod1[Pod 1 :8080]
        K8sSvc -->|Load Balance| Pod2[Pod 2 :8080]
        K8sSvc -->|Load Balance| Pod3[Pod 3 :8080]

        subgraph Pod [Pod Architecture]
            direction TB
            Mux[HTTP ServeMux<br/>Go 1.22 Routing] --> API[API Layer: internal/api]
            API --> Probes["Health & Readiness<br/>(/healthz, /readyz)"]
            API --> Metrics["Metrics<br/>(/metrics expvar)"]
            API --> Store["Store: internal/store<br/>sync.RWMutex + map[string]entry"]
            Janitor["Background Janitor<br/>(Periodic sweep goroutine)"] -->|Lock & Evict| Store
        end
    end
```

### Component Breakdown
- **`internal/store`**: Thread-safe in-memory store guarded by `sync.RWMutex`. Stores values defensively copied on both ingress and egress. Supports per-key TTL with dual-mode eviction: **lazy eviction** on read access and **periodic active sweeping** via a background janitor goroutine. Features an injectable clock interface for deterministic time-travel unit tests.
- **`internal/api`**: REST API layer leveraging Go 1.22 method-based `http.ServeMux` path matching. Enforces a strict 1 MiB value payload cap (`http.MaxBytesReader`), manages dynamic readiness states, and registers `/healthz`, `/readyz`, and `/metrics`.
- **`cmd/server`**: Service entrypoint configuring HTTP server timeouts (`ReadHeaderTimeout: 5s`, `ReadTimeout: 10s`, `WriteTimeout: 10s`), OS signal interception (`SIGINT`, `SIGTERM`), and a 3-stage graceful draining sequence designed for zero-downtime rolling updates in Kubernetes.

---

## 2. Status & Verification Matrix

| Component / Feature | Status | Verification Mechanism |
| :--- | :--- | :--- |
| **In-Memory Store (`sync.RWMutex`)** | **Implemented & Tested** | Unit tests (`store_test.go`), race detector (`-race`), concurrent stress tests |
| **Per-Key TTL & Lazy Eviction** | **Implemented & Tested** | Deterministic mock clock unit tests (`TestTTLExpiry`) |
| **Active Background Janitor** | **Implemented & Tested** | Cancellation test (`TestRunJanitorStopsOnCancel`), periodic sweep test (`TestRunJanitorPeriodicSweep`) |
| **Defensive Byte Copying** | **Implemented & Tested** | Slice mutation safety tests (`TestValueIsCopied`) |
| **HTTP CRUD API (`PUT`, `GET`, `DELETE`)** | **Implemented & Tested** | Integration test suite (`api_test.go`) |
| **Key Enumeration (`GET /v1/keys`)** | **Implemented & Tested** | Alphabetical sorting & expired-key exclusion tests (`TestListKeys`, `TestKeysReturnsActiveKeys`) |
| **Health & Readiness (`/healthz`, `/readyz`)**| **Implemented & Tested** | Lifecycle transitions tested (`TestReadinessTransitions`) |
| **Expvar Metrics (`/metrics`)** | **Implemented & Tested** | Expvar counters verified (`TestMetricsEndpoint`) |
| **Graceful Shutdown & Draining** | **Implemented & Tested** | Signal context, 3s endpoint propagation pause, 10s shutdown deadline |
| **Store Performance Benchmark Suite** | **Implemented & Tested** | Benchmarks recorded (`internal/store/store_benchmark_test.go`, [`docs/BENCHMARKS.md`](docs/BENCHMARKS.md)) |
| **Docker Multi-Stage Packaging** | **Implemented & Tested (CI)** | Alpine build -> Distroless static non-root verified in GitHub Actions |
| **Kubernetes 3-Replica Deployment** | **Manifests Implemented** | Manifests defined in `deploy/k8s/` (Deployment, Service, 3-node Kind cluster config) |
| **CI/CD Automation** | **Verified Active** | GitHub Actions workflow passing (`go vet`, `go test -race`, `docker build`) |

---

## 3. API Reference

All requests and responses use standard HTTP semantics. Errors are returned as JSON objects: `{"error": "message"}`.

### 3.1 Store a Key (`PUT /v1/keys/{key}`)
Stores a binary or text value under `{key}` with an optional time-to-live.

- **URL:** `/v1/keys/{key}?ttl={seconds}`
- **Method:** `PUT`
- **Query Parameters:**
  - `ttl` *(optional)*: Positive integer in seconds. If omitted or `0`, the key never expires.
- **Request Body:** Raw binary payload (max 1 MiB).
- **Responses:**
  - `204 No Content`: Key stored successfully.
  - `400 Bad Request`: Invalid TTL parameter.
  - `413 Request Entity Too Large`: Body exceeds 1 MiB.

```powershell
# Store key "session-token" expiring in 60 seconds
Invoke-RestMethod -Method Put -Uri "http://localhost:8080/v1/keys/session-token?ttl=60" -Body "xyz123abc"
```

### 3.2 Retrieve a Key (`GET /v1/keys/{key}`)
Retrieves the value for `{key}`.

- **URL:** `/v1/keys/{key}`
- **Method:** `GET`
- **Responses:**
  - `200 OK`: Raw binary/text payload (`Content-Type: application/octet-stream`).
  - `404 Not Found`: Key does not exist or has expired.

```powershell
Invoke-RestMethod -Method Get -Uri "http://localhost:8080/v1/keys/session-token"
```

### 3.3 Delete a Key (`DELETE /v1/keys/{key}`)
Removes `{key}` from the store.

- **URL:** `/v1/keys/{key}`
- **Method:** `DELETE`
- **Responses:**
  - `204 No Content`: Key existed and was deleted.
  - `404 Not Found`: Key was not found.

```powershell
Invoke-RestMethod -Method Delete -Uri "http://localhost:8080/v1/keys/session-token"
```

### 3.4 List Active Keys (`GET /v1/keys`)
Returns an alphabetically sorted JSON array of all active, non-expired keys. Expired keys encountered during enumeration are cleaned up automatically.

- **URL:** `/v1/keys`
- **Method:** `GET`
- **Responses:**
  - `200 OK`: `application/json`
    ```json
    {
      "keys": ["apple", "session-token", "user:1001"]
    }
    ```

```powershell
Invoke-RestMethod -Method Get -Uri "http://localhost:8080/v1/keys"
```

### 3.5 Health & Operational Endpoints

| Endpoint | Method | Purpose | Response |
| :--- | :--- | :--- | :--- |
| `/healthz` | `GET` | **Liveness Probe**: Returns 200 as long as the process is alive. | `200 OK` |
| `/readyz` | `GET` | **Readiness Probe**: Returns 200 when ready to receive traffic; returns 503 during graceful shutdown. | `200 OK` / `503 Service Unavailable` |
| `/metrics` | `GET` | **Metrics**: Exposes runtime and service counters via Go's standard `expvar`. | `200 OK` (`requests_total`, `errors_total`) |

---

## 4. Concurrency & TTL Architecture

### Dual-Eviction Strategy
1. **Lazy Eviction on Read (`Get`, `Keys`)**:
   When `Get` or `Keys` is invoked, the timestamp of each inspected key is compared against `s.now()`. If expired, the entry is deleted immediately before returning, preventing expired data leaks.
2. **Periodic Sweep (`RunJanitor`)**:
   A lightweight background goroutine runs on a configurable ticker (default 30 seconds). It acquires an exclusive lock, sweeps all expired entries in $O(N)$ time, and frees memory so keys that are never read again do not leak RAM.

### Concurrency Guarantees
- Guarded by `sync.RWMutex`: multiple concurrent readers (`Get`) execute in parallel. Writes (`Set`, `Delete`, `Keys`) acquire exclusive access.
- **Defensive Copying**: Go slices share underlying array buffers. To prevent data races between callers and the store, `Store.Set` creates an independent copy of incoming byte slices (`append([]byte(nil), value...)`), and `Store.Get` returns a newly allocated copy.
- **Race Detector Tested**: The codebase is verified clean under `go test -race`.

---

## 5. Graceful Draining & Kubernetes Lifecycle

```mermaid
sequenceDiagram
    participant K8s as Kubernetes Kubelet / SIGTERM
    participant EP as Endpoint Controller
    participant Srv as ttl-kv Server (:8080)
    participant Janitor as Janitor Goroutine

    K8s->>Srv: Sends SIGTERM / SIGINT
    Srv->>Srv: Context cancelled (ctx.Done())
    Srv->>Janitor: ctx.Done() terminates Janitor ticker
    Srv->>Srv: srv.SetReady(false)
    Note over Srv: /readyz now returns 503
    EP->>Srv: Probes /readyz (fails)
    EP->>EP: Removes Pod IP from Service Endpoints
    Note over Srv: 3-second sleep allows in-flight TCP & iptables rules to settle
    Srv->>Srv: httpSrv.Shutdown(10s timeout)
    Note over Srv: Drains active requests, stops listener
    Srv->>K8s: Process exits cleanly (0)
```

1. **Signal Interception**: Go's `signal.NotifyContext` intercepts `SIGINT` (Ctrl+C) and `SIGTERM` (Kubernetes pod termination).
2. **Readiness Flips to Unhealthy**: `srv.SetReady(false)` causes `/readyz` to immediately return `503 Service Unavailable`.
3. **Endpoint Deregistration Window (3s)**: Kubelet readiness probe fails, prompting Kubernetes to remove the Pod IP from the Service endpoints list. The 3-second sleep ensures traffic in flight through kube-proxy is not dropped.
4. **Janitor Teardown**: The Janitor goroutine terminates on `<-ctx.Done()`, stopping its ticker.
5. **Connection Draining (10s deadline)**: `httpSrv.Shutdown` stops accepting new connections on `:8080` and permits existing requests to complete cleanly.
6. **Aligned Termination Grace Period**: The 3s sleep + 10s shutdown context = 13s, safely within the Kubernetes `terminationGracePeriodSeconds: 20`.

---

## 6. Performance Benchmarks

Benchmarks were performed on a 12-core host (`12th Gen Intel Core i5-1235U`) running Go 1.27 on Windows 11. See [`docs/BENCHMARKS.md`](docs/BENCHMARKS.md) for full methodology and details.

| Benchmark Operation | Latency | Single-Thread Throughput | Allocations | Memory Overhead |
| :--- | :--- | :--- | :--- | :--- |
| **`StoreSet`** (256 B payload) | **121.3 ns/op** | **2,110 MB/s (~8.2M ops/s)** | 1 alloc/op | 256 B/op (defensive copy) |
| **`StoreGet`** (256 B payload) | **117.5 ns/op** | **2,179 MB/s (~8.5M ops/s)** | 1 alloc/op | 256 B/op (defensive copy) |
| **`StoreDelete`** | **265.0 ns/op** | **~3.7M ops/s** | **0 alloc/op** | **0 B/op** |
| **`StoreKeys`** (10 keys) | **380.9 ns/op** | N/A | 1 alloc/op | 160 B/op |
| **`StoreKeys`** (100 keys) | **6.69 µs/op** | N/A | 1 alloc/op | 1,792 B/op |
| **`StoreKeys`** (1,000 keys) | **102.0 µs/op** | N/A | 1 alloc/op | 16,384 B/op |
| **`StoreParallel`** (12-thread contention) | **429.5 ns/op** | **596 MB/s** | 1 alloc/op | 256 B/op |
| **`StoreMixedWorkload`** (80% R / 15% W / 5% D)| **354.6 ns/op** | **723 MB/s** | 0 alloc/op | 193 B/op |

To run the benchmarks locally:
```powershell
go test -run='^$' -bench='.' -benchmem ./internal/store
```

---

## 7. Local Development & Testing Guide

### 7.1 Running Tests
```powershell
# Format check
go fmt ./...

# Static analysis
go vet ./...

# Run all unit tests
go test -v -count=1 ./...

# Run tests with race detector (requires GCC on Windows / runs in Linux container or CI)
go test -race -count=1 ./...
```

### 7.2 Running the Service Locally
```powershell
# Runs server listening on default :8080 (or custom ADDR environment variable)
$env:ADDR=":8080"
go run ./cmd/server
```

---

## 8. Docker Packaging

The service uses a hardened two-stage Docker build:
1. **Builder Stage (`golang:1.22-alpine`)**: Builds a static Linux binary (`CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w"`).
2. **Runtime Stage (`gcr.io/distroless/static-debian12:nonroot`)**: Distroless minimal image containing zero package managers or shells. Runs unprivileged as `nonroot:nonroot` (UID 65532). Total compressed image size is under 15 MB.

### Build and Run with Docker
```powershell
# Build image
docker build -t ttl-kv:dev .

# Run container exposing port 8080
docker run --rm -d --name ttl-kv-test -p 8080:8080 ttl-kv:dev

# Test endpoints
Invoke-RestMethod -Uri "http://localhost:8080/healthz"
Invoke-RestMethod -Uri "http://localhost:8080/readyz"
Invoke-RestMethod -Method Put -Uri "http://localhost:8080/v1/keys/ping" -Body "pong"
Invoke-RestMethod -Uri "http://localhost:8080/v1/keys/ping"

# Stop container
docker stop ttl-kv-test
```

---

## 9. Kubernetes Deployment

The service includes production Kubernetes manifests under `deploy/k8s/`:
- **`deploy/k8s/deployment.yaml`**: 3 replicas, zero-downtime rolling update strategy (`maxUnavailable: 0`, `maxSurge: 1`), resource requests/limits, HTTP liveness (`/healthz`) and readiness (`/readyz`) probes, non-root security context, read-only root filesystem.
- **`deploy/k8s/service.yaml`**: `Service` exposing port 80 mapping to container port 8080.
- **`deploy/k8s/kind-config.yaml`**: Local 3-node Kind cluster topology (1 control plane, 2 worker nodes).

### Deploying to a Local Kind Cluster
Prerequisites: Docker Desktop running, `kind`, and `kubectl` installed.

```powershell
# 1. Create 3-node Kind cluster
kind create cluster --name ttl-kv --config deploy/k8s/kind-config.yaml

# 2. Build local Docker image
docker build -t ttl-kv:dev .

# 3. Load image into Kind nodes
kind load docker-image ttl-kv:dev --name ttl-kv

# 4. Apply manifests
kubectl apply -f deploy/k8s/

# 5. Monitor rollout status
kubectl rollout status deployment/ttl-kv

# 6. Verify pods and service
kubectl get pods -o wide
kubectl get services

# 7. Forward port to test
kubectl port-forward svc/ttl-kv 8080:80
```

---

## 10. Design Decisions & Architectural Limitations

> [!IMPORTANT]
> **Independent Replicas (No Shared State)**:
> In the current architecture, every pod replica maintains an **independent in-memory store**. A key written to Pod A through the round-robin Kubernetes Service will not be present on Pod B. This design represents a standalone in-memory node architecture and does not claim distributed consistency.

1. **Single Global Lock (`sync.RWMutex`)**:
   - Chosen for correctness, simplicity, and zero edge-case concurrency bugs under the Go race detector.
   - *Trade-off*: High write volume serializes on the mutex lock. For extreme write scaling, a striped/sharded map lock would improve throughput.
2. **Dual-Tier Eviction (Lazy + Periodic)**:
   - Avoids creating individual timers (`time.AfterFunc`) per key, preventing goroutine and memory leaks when millions of keys are inserted.
3. **Defensive Byte Copying**:
   - Ensures caller modifications to buffers cannot corrupt the internal store.
   - Incurs a minor 1-allocation overhead per read/write, measured at ~117–121 ns per operation in benchmarks.
4. **Ephemerality**:
   - Data resides strictly in RAM. Process termination or pod eviction clears all keys.

---

## 11. Planned Future Improvements

1. **Consistent Hashing / Partitioning**: Implement a consistent hash ring across pods or deploy via StatefulSet with a routing proxy (e.g., Envoy) so keys deterministically map to specific pods.
2. **Distributed Replication**: Add Raft-based consensus or leader-follower asynchronous replication for cluster-wide consistency.
3. **Lock Striping**: Shard the internal map into 32 or 64 independent mutex buckets to reduce lock contention under parallel writes.
4. **Prometheus OpenMetrics**: Expose Prometheus-compatible `/metrics` scraping format alongside the existing expvar counters.
