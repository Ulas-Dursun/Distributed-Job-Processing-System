# ⚡ Distributed Job Processing System

A distributed job processing system built with Go. Submit a URL via the web UI or REST API — a worker pool fetches the page, counts words, and persists the result. Includes retry logic, graceful shutdown, Prometheus metrics, and Docker Compose for one-command deployment.

![Go](https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-blue?style=flat-square&logo=postgresql)
![Redis](https://img.shields.io/badge/Redis-7-red?style=flat-square&logo=redis)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker)
![Prometheus](https://img.shields.io/badge/Prometheus-Metrics-E6522C?style=flat-square&logo=prometheus)

---

## ✨ Features

- **Concurrent worker pool**  
  Configurable goroutine count via environment variable.

- **Exponential backoff retry**  
  Retries failed jobs up to 3 times before marking them as `FAILED`.

- **Graceful shutdown**  
  Handles `SIGTERM` cleanly and drains in-flight jobs before exit.

- **Prometheus metrics**  
  Tracks job counts, processing duration, and queue size.

- **Web UI**  
  Live polling dashboard with pipeline step visualization.

- **Docker Compose deployment**  
  Full stack runs with a single command.

---

## How It Works

```text
Client
  │
  ├── POST /api/jobs
  │
  ▼
PostgreSQL
  │
  ├── job inserted as QUEUED
  │
  ▼
Redis Queue
  │
  ├── job ID enqueued via LPUSH
  │
  ▼
Worker Pool (5 goroutines by default)
  │
  ├── BRPOP job ID
  ├── fetch URL
  ├── count words
  └── update PostgreSQL as COMPLETED or FAILED
````

## 🚀 Quick Start

### Prerequisites

* Go 1.23+
* PostgreSQL
* Redis

### Local Development

```bash
git clone https://github.com/ulasdursun/queueforge.git
cd queueforge
cp .env.example .env
go run ./cmd/server
```

Open:

```text
http://localhost:8080
```

### Docker

```bash
docker-compose up --build
```

## 📍 Services

| Service    | URL                                            |
| ---------- | ---------------------------------------------- |
| Web UI     | [http://localhost:8080](http://localhost:8080) |
| Prometheus | [http://localhost:9090](http://localhost:9090) |
| Grafana    | [http://localhost:3000](http://localhost:3000) |

Grafana default credentials: `admin / admin`

## API

### Create a Job

```bash
curl -X POST http://localhost:8080/api/jobs \
  -H "Content-Type: application/json" \
  -d '{"type":"URL_FETCH","payload":"{\"url\":\"https://nytimes.com\"}"}'
```

### Endpoints

| Method | Endpoint        | Description                            |
| ------ | --------------- | -------------------------------------- |
| `POST` | `/api/jobs`     | Create a URL fetch job                 |
| `GET`  | `/api/jobs`     | List jobs, optionally filter by status |
| `GET`  | `/api/jobs/:id` | Get job details by ID                  |
| `GET`  | `/health`       | Health check                           |
| `GET`  | `/metrics`      | Prometheus metrics                     |

## ⚙️ Configuration

Environment variables:

| Variable       | Default | Description                    |
| -------------- | ------- | ------------------------------ |
| `SERVER_PORT`  | `8080`  | HTTP server port               |
| `DATABASE_URL` | —       | PostgreSQL connection URL      |
| `REDIS_URL`    | —       | Redis connection URL           |
| `WORKER_COUNT` | `5`     | Number of worker goroutines    |
| `MAX_RETRIES`  | `3`     | Maximum retry attempts per job |

## 🧱 Project Structure

```text
queueforge/
├── cmd/server/        # Application entry point and wiring
├── config/            # Environment variable loading
├── internal/
│   ├── api/           # Gin router, handlers, middleware
│   ├── domain/        # Job entity, status enum, errors
│   ├── metrics/       # Prometheus counters and histograms
│   ├── queue/         # Redis push/pop layer
│   ├── repository/    # PostgreSQL queries
│   └── worker/        # Worker pool, processor, retry logic
├── static/            # Web UI
├── docker/            # init.sql, prometheus.yml, Grafana config
├── Dockerfile
├── docker-compose.yml
└── go.mod
```

## 🧠 Engineering Decisions

### Why PostgreSQL for job state?

PostgreSQL provides durable state for job lifecycle tracking. Even if Redis loses queued entries, the system still preserves job records and final status in the database.

### Why Redis for the queue?

Redis gives fast enqueue/dequeue operations and keeps the worker pipeline simple and efficient.

### Why a worker pool?

A fixed worker pool gives predictable concurrency and makes throughput easy to control through `WORKER_COUNT`.

### Why retries with backoff?

Network requests are inherently unstable. Exponential backoff reduces pressure on transient failures and improves job completion reliability.

### Why graceful shutdown?

It prevents job loss during deploys or container restarts by letting in-flight work finish cleanly.

## ⚠️ Known Limitations

* Redis restarts can lose queued job IDs, while job records remain in PostgreSQL as `QUEUED`.
* The system currently runs as a single worker node and does not support horizontal worker scaling.
* Job type is currently limited to `URL_FETCH`.

## 📈 Observability

QueueForge exposes Prometheus metrics for:

* total job counts
* job processing duration
* queue size

A Grafana dashboard can be used to visualize system behavior over time.
## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
