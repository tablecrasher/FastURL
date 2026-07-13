# FastURL

FastURL is a high-performance URL shortener built in Go. It generates deterministic short codes with SHA-256 and Base32, stores mappings in PostgreSQL, serves redirects through a Redis cache-aside layer, and tracks click analytics asynchronously so the redirect path stays fast.

---

## Request Flow

```
POST /shorten ──> hash URL ──> INSERT into Postgres (ON CONFLICT DO NOTHING) ──> return short URL

GET /{code} ──> check Redis ──> hit?  redirect immediately
                             └─> miss? query Postgres ──> fill Redis (1h TTL) ──> redirect
                                         │
                                         └─> INCR clicks:{code} in Redis (async analytics)
```

---

## Short Code Generation

Codes are deterministic: the same URL always produces the same short code.

1. SHA-256 hash of the input URL
2. Take the first 5 bytes
3. Base32 encode → exactly 8 characters, no padding

Base32's alphabet (A–Z, 2–7) is URL-safe and case-insensitive, with no ambiguous characters like `0/O` or `1/l`. Determinism means duplicate submissions return the same short link instead of creating new entries — and 5 bytes = 40 bits = 8 clean Base32 characters with zero `=` padding.

---

## PostgreSQL

Postgres is the source of truth. The `code` column is the primary key, which does double duty:

- **Fast lookups** — redirects query by code, and primary keys are indexed
- **Collision protection** — if two different URLs ever hashed to the same code, the insert fails loudly instead of silently overwriting an existing link

Inserts use `ON CONFLICT (code) DO NOTHING`, so concurrent duplicate requests are resolved atomically by the database — no check-then-act race conditions.

---

## Redis Cache

Redirects are the hot path, so reads go through a cache-aside layer:

1. Check Redis for the code
2. **Hit** — redirect immediately, Postgres never touched
3. **Miss** — query Postgres, fill Redis with a 1-hour TTL, redirect

Cache fills are best-effort: if the `SET` fails, the user still gets their redirect and the system just pays a cache miss next time.

---

## Fallback

If Redis goes down, reads fall through to Postgres directly. A cache outage makes the system slower, not broken — every request is treated as a cache miss and answered by the source of truth. Redis errors are logged so an unhealthy cache is visible, but they never fail a redirect.

---

## Click Analytics

Counting clicks with a Postgres `UPDATE` on every redirect would put a database write on the hottest path. Instead, hot and cold paths are decoupled:

- **Hot path** — each redirect runs an atomic `INCR clicks:{code}` in Redis (sub-millisecond, no locks)
- **Cold path** — a background goroutine wakes on a ticker, drains every `clicks:*` counter with an atomic `GETDEL`, and adds the totals to the `clicks` column in Postgres

`GETDEL` reads and deletes in one atomic operation, so a concurrent click can never land between the read and the delete and get lost.

---

## Project Structure

```
FastURL/
├── main.go       # Connects to Postgres and Redis, wires routes, starts the server
├── store.go      # Data layer: Postgres queries, cache-aside logic, click flusher
├── handlers.go   # HTTP layer: request parsing, validation, response codes
├── go.mod
└── go.sum
```

The store knows about SQL and Redis; the handlers know about HTTP. Neither touches the other's concerns.

---

## Getting Started

**1. Start Postgres and Redis (requires Docker):**

```bash
docker run -d --name shortener-db -p 5432:5432 -e POSTGRES_PASSWORD=devpass postgres:16
docker run -d --name shortener-cache -p 6379:6379 redis:7
```

**2. Create the schema:**

```bash
docker exec -it shortener-db psql -U postgres
```

```sql
CREATE TABLE urls (
    code       VARCHAR(8) PRIMARY KEY,
    long_url   TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    clicks     BIGINT NOT NULL DEFAULT 0
);
```

**3. Run the server:**

```bash
go run .
```

**4. Shorten a URL:**

```bash
curl -X POST localhost:8080/shorten -d '{"url":"https://google.com"}'
# {"short_url":"http://localhost:8080/AUCG6JWI"}
```

**5. Follow the redirect:**

```bash
curl -i localhost:8080/AUCG6JWI
# HTTP/1.1 302 Found
# Location: https://google.com
```

**6. Watch the cache work (optional):**

```bash
docker exec -it shortener-cache redis-cli MONITOR
```

Hit the same short link twice — the first request shows a `GET` miss followed by a `SET`, the second shows only a `GET`. That silent second request is Postgres being skipped.

**7. Check click analytics:**

```bash
docker exec -it shortener-db psql -U postgres -c "SELECT code, clicks FROM urls;"
```

Counters accumulate in Redis and are flushed to Postgres by the background worker every 10 seconds.

---

## API

| Endpoint | Method | Description |
|---|---|---|
| `/shorten` | POST | Body: `{"url": "https://..."}`. Returns `{"short_url": "..."}`. `400` on invalid or empty URL. |
| `/{code}` | GET | `302` redirect to the original URL. `404` if the code doesn't exist. |

---

## Design Decisions

| Decision | Why |
|---|---|
| Deterministic hashing over random codes | Same URL → same link; no duplicate entries; cache-friendly |
| `code` as primary key | Indexed lookups on the hot path + loud failure on hash collisions |
| `ON CONFLICT DO NOTHING` | Concurrent duplicate inserts resolved atomically by the database |
| 302 over 301 redirects | Browsers cache 301s permanently, which would make click analytics impossible |
| Cache errors never fail requests | Postgres is the source of truth; Redis down = slower, not broken |
| `GETDEL` for counter draining | Atomic read+delete — clicks can't be lost between the two steps |

---

## What's Next

- **Link expiration** — `expires_at` column and a `WHERE` clause on lookups
- **Rate limiting** — Redis `INCR` with TTL per client IP
- **Custom aliases** — user-chosen codes with reserved-word and collision handling
- **Geographic distribution** — regional Redis and app instances to cut global latency
