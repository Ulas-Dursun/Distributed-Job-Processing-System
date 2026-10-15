# ⚡ QueueForge

A robust, concurrent background job processing system built with Go. Submit a URL via the modern web dashboard or REST API — an in-process or standalone worker pool fetches the page, counts words, and persists the execution state to PostgreSQL and Redis.

Designed with enterprise-grade reliability patterns: service decoupling (`APP_MODE=api|worker`), non-blocking exponential backoff retries, two-phase graceful shutdown with sync barriers, SSRF security hardening, pre-provisioned Prometheus/Grafana observability, and one-command Docker Compose orchestration.

![Go](https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-blue?style=flat-square&logo=postgresql)
![Redis](https://img.shields.io/badge/Redis-7-red?style=flat-square&logo=redis)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker)
![Prometheus](https://img.shields.io/badge/Prometheus-Metrics-E6522C?style=flat-square&logo=prometheus)
![License](https://img.shields.io/badge/License-MIT-green.svg?style=flat-square)

---

## 🏗️ Architecture Flow

```text
Client (Web UI / REST API)
       │
       ├── POST /api/jobs (Input Validation & SSRF Guard)
       │
       ▼
PostgreSQL (QUEUED) ──[Fail-safe Dual Write]──► Redis Queue (LPUSH)
                                                    │
                                                    ▼
                                            Worker Pool (BRPOP)
                                                    │
                                     ┌──────────────┴──────────────┐
                                     ▼                             ▼
                            Worker Goroutine 1           Worker Goroutine N
                                     │
                             ├── SSRF Validation (Private IP / Metadata block)
                             ├── Bounded HTTP Fetch (2MB max stream reader)
                             ├── Word Count Processing
                             └── Persist Result (COMPLETED / FAILED)
                                     │
                             [On Failure] ──► Non-blocking Exponential Backoff (Re-queue)
```

## ✨ Core Features

- **Decoupled Architecture (`APP_MODE`)** — Run as a unified monolith (`all`), dedicated API gateway (`api`), or autonomous background worker (`worker`).
- **Concurrent Worker Pool** — Configurable goroutine count with panic recovery and `sync.WaitGroup` barrier tracking.
- **Horizontal Worker Scalability** — Seamlessly scale consumers across multiple containers (`docker compose up --scale worker=3`).
- **Non-Blocking Exponential Backoff** — Failed jobs are re-queued after exponential delay without blocking worker threads from processing active items.
- **Two-Phase Graceful Shutdown** — Shuts down the HTTP server first to stop incoming traffic, then safely drains in-flight worker executions.
- **Security Hardening (SSRF & OOM Protection)** — Rejects loopback, private RFC 1918, and AWS metadata IP ranges; enforces a 2MB stream limit on HTTP reads.
- **Pre-Provisioned Observability** — Out-of-the-box Grafana dashboard and Prometheus scraping (queue depth, active worker gauges, latency percentiles).
- **Polished Web UI** — Real-time execution tracking, pipeline stage visualization, live event logs, and interactive historical job inspection.
- **Automated CI & Testing** — GitHub Actions CI pipeline running formatting, vetting, race-detector unit tests, and load testing scripts.

---

## 📍 Services

| Service    | URL                                            | Default Credentials |
| ---------- | ---------------------------------------------- | ------------------- |
| Web UI     | [http://localhost:8080](http://localhost:8080) | —                   |
| Prometheus | [http://localhost:9090](http://localhost:9090) | —                   |
| Grafana    | [http://localhost:3000](http://localhost:3000) | `admin / admin`     |

---

## 🚀 Quick Start

### 1. Run with Docker Compose (Recommended)

```bash
docker compose up --build -d
```

To scale background workers horizontally:
```bash
docker compose up --scale worker=3 -d
```

### 2. Local Development

Requires Go 1.23+, PostgreSQL 16+, and Redis 7+.

```bash
git clone https://github.com/ulasdursun/queueforge.git
cd queueforge
cp .env.example .env
go run ./cmd/server
```

Open [http://localhost:8080](http://localhost:8080) in your browser.

---

## 🛠️ Developer Automation (Makefile)

```bash
make test         # Run unit tests
make vet          # Run static code analysis
make docker-up    # Start full stack in Docker
make docker-scale # Start and scale to 3 worker instances
make load-test    # Run automated 500-submission load test
make docker-down  # Stop all services
```

---

## 🧪 Testing & Verification

### Unit Tests
```bash
go test -v ./...
```

### Automated Load Testing
QueueForge includes a dedicated concurrent load testing CLI:

```bash
# Submit 500 jobs with 50 concurrent workers
go run ./scripts/load_test.go -url http://localhost:8080 -c 50 -n 500
```

Sample output:
```text
🚀 Starting QueueForge Load Test
   Target Endpoint: http://localhost:8080/api/jobs
   Total Submissions: 500
   Concurrency Level: 50 workers

📊 Load Test Summary Results:
   Throughput:         482.10 req/sec
   Successful (201):   500 (100.0%)
⏱  Submission Latency Percentiles:
   p50 (Median):       18ms
   p95:                42ms
   p99:                74ms
```

---

## 📡 REST API Reference

| Method | Endpoint        | Description                                    |
| ------ | --------------- | ---------------------------------------------- |
| `POST` | `/api/jobs`     | Create a URL fetch job                         |
| `GET`  | `/api/jobs`     | List jobs (`?status=QUEUED&limit=50&offset=0`) |
| `GET`  | `/api/jobs/:id` | Get job details by UUID                        |
| `GET`  | `/health`       | Service health status                          |
| `GET`  | `/health/live`  | Kubernetes Liveness probe                      |
| `GET`  | `/health/ready` | Dependency Readiness probe (PostgreSQL, Redis) |
| `GET`  | `/metrics`      | Prometheus metrics exporter                    |

### Example: Create Job
```bash
curl -X POST http://localhost:8080/api/jobs \
  -H "Content-Type: application/json" \
  -d '{"type":"URL_FETCH","payload":"{\"url\":\"https://example.com\"}"}'
```

---

## ⚙️ Configuration

Environment variables configured via `.env`:

| Variable            | Default | Description                                              |
| ------------------- | ------- | -------------------------------------------------------- |
| `APP_MODE`          | `all`   | Execution mode: `all` (monolith), `api`, or `worker`     |
| `SERVER_PORT`       | `8080`  | HTTP server port                                         |
| `DATABASE_URL`      | —       | PostgreSQL connection URL                                |
| `REDIS_URL`         | —       | Redis connection URL                                     |
| `WORKER_COUNT`      | `5`     | Number of parallel worker goroutines per instance        |
| `MAX_RETRIES`       | `3`     | Maximum retry attempts per failed job                    |
| `ALLOW_LOCAL_FETCH` | `false` | Security override (set `true` ONLY in automated mock tests) |

---

## 🧱 Project Structure

```text
queueforge/
├── cmd/server/        # Application entrypoint & graceful shutdown orchestration
├── config/            # Environment variable parsing (godotenv)
├── internal/
│   ├── api/           # Gin router, handlers, middleware & health probes
│   ├── domain/        # Core domain entities, status enums, errors
│   ├── metrics/       # Custom Prometheus counters, gauges, histograms
│   ├── queue/         # Redis queue operations (LPUSH, BRPOP, LLEN)
│   ├── repository/    # PostgreSQL data access layer & auto-schema migration
│   └── worker/        # Goroutine worker pool, SSRF guard & HTTP processor
├── static/            # Responsive web dashboard with live polling & recent history
├── docker/            # PostgreSQL init.sql, Prometheus & Grafana provisioning
├── scripts/           # Concurrent load test CLI tool
├── Dockerfile         # Multi-stage minimal container build
├── docker-compose.yml # Decoupled multi-service orchestration
└── Makefile           # Developer automation tasks
```

---

## 🧠 Engineering Decisions

- **Why PostgreSQL for state?** Provides durable persistence and queryable history. Even if Redis restarts, job history and results remain intact.
- **Why Redis for queueing?** Low-latency atomic `LPUSH`/`BRPOP` operations ensure rapid job dispatch without database locking contention.
- **Why in-process worker pool?** Fixed-size goroutine pools provide predictable memory footprint and prevent unbounded concurrency spikes.
- **Why non-blocking backoff?** Re-queuing failed jobs via asynchronous timers prevents worker goroutine starvation, ensuring healthy jobs continue executing.
- **Why SSRF and stream limits?** Fetching arbitrary user-provided URLs requires strict private subnet filtering (RFC 1918 / cloud metadata) and bounded memory reads to prevent server compromise or OOM crashes.

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
