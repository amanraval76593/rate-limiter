# Rate-Limited API

`rate-limited-api` is a small Go service that protects an endpoint with a Redis-backed sliding-window rate limiter.
When a request is blocked, the service returns `429 Too Many Requests` and also places the payload into a retry queue that is processed by a background worker.

The project is intentionally compact, but it still demonstrates a few useful backend patterns:

- Redis Lua scripting for atomic rate-limit checks
- A retry queue backed by a Redis sorted set
- Per-user request statistics
- Graceful shutdown for both the HTTP server and background worker

## What The Service Does

- Accepts requests on `POST /request`
- Applies a per-user rate limit of `5` requests per `60` seconds
- Returns `200 OK` for allowed requests
- Returns `429 Too Many Requests` with `retry_after` for blocked requests
- Enqueues blocked requests for later retry
- Exposes per-user stats on `GET /stats?user_id=<id>`
- Exposes a simple health endpoint on `GET /health`

## Tech Stack

- Go
- Gin
- Redis
- Redis Lua script for atomic limiter logic
- Docker Compose for local Redis

## Project Structure

```text
.
├── cmd/main.go                      # Application bootstrap
├── api/routes.go                    # HTTP route registration
├── internal/handler/                # HTTP handlers
├── internal/service/request_service.go
├── internal/limiter/redis_limiter.go
├── internal/worker/retry_worker.go
├── internal/redis/client.go         # Redis client setup
├── internal/model/request.go        # Request/response models
├── scripts/limiter.lua              # Atomic sliding-window script
└── docker-compose.yml               # Local Redis
```

## Steps To Run The Project

### Prerequisites

- Go installed locally
- Docker and Docker Compose available

This repo currently declares `go 1.25.0` in `go.mod`, so using the matching Go toolchain is the safest path.

### 1. Start Redis

```bash
docker compose up -d
```

This starts Redis on `localhost:6382`.

### 2. Run The API

```bash
go run ./cmd
```

The API starts on `http://localhost:8081`.

### 3. Verify The Service

Health check:

```bash
curl http://localhost:8081/health
```

Expected response:

```json
{"status":"ok"}
```

### 4. Send Requests

Sample allowed request:

```bash
curl -X POST http://localhost:8081/request \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user-123",
    "payload": {
      "message": "hello"
    }
  }'
```

Success response:

```json
{"status":"success"}
```

If the same user exceeds the limit, the service responds like this:

```json
{
  "error": "rate limit exceeded",
  "retry_after": 37
}
```

### 5. View Per-User Stats

```bash
curl "http://localhost:8081/stats?user_id=user-123"
```

Example response:

```json
{
  "user_id": "user-123",
  "total_requests": 7,
  "allowed": 5,
  "blocked": 2
}
```

## Configuration

The service reads Redis configuration from environment variables:

- `REDIS_ADDR` default: `localhost:6382`
- `REDIS_PASSWORD` default: empty
- `REDIS_DB` default: `0`

Example:

```bash
REDIS_ADDR=localhost:6382 REDIS_DB=0 go run ./cmd
```

## API Summary

### `GET /health`

Returns service status.

### `POST /request`

Request body:

```json
{
  "user_id": "user-123",
  "payload": {
    "message": "hello"
  }
}
```

Responses:

- `200 OK` when the request is allowed
- `400 Bad Request` when the JSON body is invalid
- `429 Too Many Requests` when the user has exceeded the rate limit
- `500 Internal Server Error` if processing fails

### `GET /stats?user_id=<id>`

Returns aggregated stats for the provided user.

## Design Decisions

### 1. Redis Is The Single Coordination Layer

Redis is used for:

- rate-limit counters
- queued retries
- request statistics

That keeps the service simple and avoids introducing a second persistence or queueing system for a small project.

### 2. Sliding-Window Rate Limiting Via Sorted Sets

The limiter stores request timestamps in a Redis sorted set per user. Before checking the limit, old entries are removed from the set. This gives a more accurate rolling-window limit than a fixed bucket reset model.

### 3. Lua Script For Atomicity

The rate-limit decision is executed through [`scripts/limiter.lua`](scripts/limiter.lua), which performs:

- cleanup of expired entries
- count lookup
- allow/block decision
- insertion of the current request when allowed

Doing this in Lua prevents race conditions that could happen if these steps were split across multiple Redis round trips.

### 4. Retry Queue Backed By A Redis Sorted Set

Blocked requests are placed in a sorted set named `retry_queue`, where the score is the next eligible retry time. The background worker checks the earliest item and retries it when its scheduled time arrives.

This is a lightweight choice that works well for a single-process project and keeps the queue semantics easy to understand.

### 5. Stats Are Updated Separately From Rate-Limit State

The service records per-user counters for:

- `total_requests`
- `allowed`
- `blocked`

These are updated in Redis using a pipeline to reduce round trips and keep the handler logic straightforward.

### 6. Graceful Shutdown Was Included Early

The main process cancels the worker context, shuts down the HTTP server with a timeout, and waits for the worker to stop. That is a small but important choice because background workers are often the first place shutdown bugs appear.

## What I Would Improve With More Time

### 1. Make Retry Processing More Robust

Currently the worker peeks the earliest retry item, waits for polling intervals, and then removes the entry before processing. With more time, I would improve this by:

- using a blocking or more event-driven retry pattern
- separating transient processing failures from rate-limit retries more cleanly
- making queue claiming more explicit for multi-worker safety
- storing a richer retry state and failure reason

### 2. Add Tests

The biggest gap right now is automated verification. I would add:

- unit tests for the Lua-backed limiter behavior
- handler tests for status codes and payload validation
- integration tests using Redis
- worker tests for retry scheduling and max-retry behavior

### 3. Make The Rate Limit Configurable

The current `5 requests / 60 seconds` rule is hard-coded. I would move window and limit values into configuration so the same service can support multiple environments and different traffic profiles.

### 4. Improve Observability

The service logs useful events, but I would add:

- structured logging
- metrics for allowed, blocked, retried, and dropped requests
- queue depth visibility
- request tracing or correlation IDs

### 5. Tighten The API Contract

The request payload is currently stored as `map[string]interface{}`. That is flexible, but it weakens validation. If this were moving toward production, I would either define a stricter schema or make payload validation pluggable.

## Notes

- Redis is expected to be available before the app starts.
- The retry worker runs in-process with the API.
- Blocked requests are queued for retry, but the current stats endpoint reports request counts rather than queue outcomes.
