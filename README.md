# Distributed Rate Limiter as a Service

A production-grade rate limiting microservice written in Go that exposes a gRPC API and uses Redis for distributed state. Implements two algorithms — **Token Bucket** and **Sliding Window Log** — so their behavior under load can be compared directly.

Live on Google Cloud Run: `https://ratelimiter-859547468816.us-central1.run.app`

## Stack

`Go` · `gRPC` · `Protocol Buffers` · `Redis` · `Docker` · `Google Cloud Run` · `GitHub Actions`

## Architecture

```
┌─────────────────┐        gRPC (port 50051)       ┌─────────────────────┐
│   CLI Client    │ ──────────────────────────────▶ │   Go gRPC Server    │
│  (go run ...)   │                                  │                     │
└─────────────────┘                                  │  ┌───────────────┐  │
                                                     │  │ Token Bucket  │  │
                                                     │  └───────┬───────┘  │
                                                     │          │ Lua       │
                                                     │  ┌───────▼───────┐  │
                                                     │  │Sliding Window │  │
                                                     │  └───────┬───────┘  │
                                                     └──────────┼──────────┘
                                                                │
                                                     ┌──────────▼──────────┐
                                                     │        Redis        │
                                                     │  (atomic Lua scripts│
                                                     │   no race conditions│
                                                     │   across instances) │
                                                     └─────────────────────┘
```

## Algorithms

### Token Bucket
Each client gets a bucket with a fixed capacity. Tokens refill at a constant rate. Requests consume one token — if the bucket is empty, the request is denied. Best for allowing short bursts while enforcing an average rate.

### Sliding Window Log
Tracks the exact timestamp of every request in a Redis sorted set. On each request, entries outside the window are pruned and the count is checked against capacity. Provides precise rate limiting with no boundary burst problem. More memory-intensive than token bucket.

### Key Design Decision
Both algorithms run their critical section as an **atomic Lua script inside Redis**. This means the check-and-update is a single operation — no race conditions even when multiple server instances handle requests concurrently.

## Quick Start

```bash
# Start the stack (Redis + gRPC server)
docker compose up --build

# In a new terminal — test token bucket (5 capacity, 1 token/sec refill)
go run client/main.go -algo=token -capacity=5 -rate=1 -requests=20 -client=my-client

# Test sliding window (5 requests per 5 second window)
go run client/main.go -algo=sliding -capacity=5 -window=5000 -requests=20 -client=my-client

# Benchmark with concurrent requests
go run client/main.go -algo=token -capacity=10 -rate=2 -requests=30 -client=burst-test -concurrent=true
```

## CLI Flags

| Flag | Default | Description |
|---|---|---|
| `-addr` | `localhost:50051` | gRPC server address |
| `-algo` | `token` | `token` or `sliding` |
| `-client` | `test-client` | Client identifier (e.g. IP or API key) |
| `-capacity` | `10` | Max tokens / max requests in window |
| `-rate` | `2` | Tokens refilled per second (token bucket) |
| `-window` | `5000` | Window size in milliseconds (sliding window) |
| `-requests` | `20` | Total requests to send |
| `-concurrent` | `false` | Fire all requests simultaneously |

## Project Structure

```
.
├── proto/                  # Protobuf definition + generated Go code
│   └── ratelimit.proto
├── server/
│   ├── main.go             # gRPC server entry point
│   └── algorithm/
│       ├── tokenbucket.go  # Token bucket implementation
│       ├── slidingwindow.go# Sliding window log implementation
│       └── algorithm_test.go
├── client/
│   └── main.go             # CLI client + benchmarking tool
├── docker/
│   └── Dockerfile          # Multi-stage build
├── docker-compose.yml      # Local dev stack
└── .github/workflows/
    └── ci.yml              # Build, proto check, test on every push
```

## CI/CD

GitHub Actions runs on every push to `main`:
- Installs Go and protoc
- Verifies generated proto files are up to date
- Builds the binary
- Runs both algorithm tests against a real Redis instance

## Deployment

Deployed to Google Cloud Run via Docker and Google Container Registry. Redis state is managed by Upstash (serverless Redis with TLS).

```bash
docker build -t gcr.io/bliss-rate-limiter/ratelimiter:latest -f docker/Dockerfile .
docker push gcr.io/bliss-rate-limiter/ratelimiter:latest
gcloud run deploy ratelimiter --image gcr.io/bliss-rate-limiter/ratelimiter:latest \
  --platform managed --region us-central1 --use-http2
```
