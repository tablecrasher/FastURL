# FastURL

FastURL is a URL shortener built in Go. It generates deterministic short codes with SHA-256 and Base32, stores mappings in PostgreSQL, serves redirects through a Redis cache, and tracks click analytics asynchronously so the redirect path stays fast.

---

## Request Flow

```
POST /shorten ──> hash URL ──> INSERT into Postgres ──> return short URL

GET /{code} ──> check Redis ──> hit?  redirect immediately
                             └─> miss? query Postgres ──> fill Redis (1h TTL) ──> redirect
                                         │
                                         └─> INCR clicks:{code} in Redis
```

---

## Short Codes

The same URL always produces the same code: SHA-256 the URL, take the first 5 bytes, Base32 encode → exactly 8 URL-safe characters, no padding. Duplicate submissions return the same short link instead of creating new entries.

---

## Storage

**PostgreSQL** is the source of truth. `code` is the primary key — indexed lookups on the hot path, and a loud failure instead of a silent overwrite if two URLs ever hash to the same code. Inserts use `ON CONFLICT (code) DO NOTHING`, so concurrent duplicate requests are resolved atomically by the database.

**Redis** sits in front as a cache-aside layer with a 1-hour TTL. If Redis goes down, reads fall through to Postgres — a cache outage makes the system slower, not broken.

---

## Click Analytics

Writes stay off the redirect path. Each redirect runs an atomic `INCR clicks:{code}` in Redis; a background goroutine drains the counters with `GETDEL` on a ticker and adds the totals to Postgres. `GETDEL` reads and deletes atomically, so clicks can't be lost between the two steps.

---

## Project Structure

```
FastURL/
├── main.go       # Connects to Postgres and Redis, wires routes, starts the server
├── store.go      # Data layer: Postgres queries, cache-aside logic, click flusher
└── handlers.go   # HTTP layer: request parsing, validation, response codes
```

---

## Getting Started

**1. Start Postgres and Redis (requires Docker):**

```bash
docker run -d --name shortener-db -p 5432:5432 -e POSTGRES_PASSWORD=devpass postgres:16
docker run -d --name shortener-cache -p 6379:6379 redis:7
```

**2. Create the schema** (`docker exec -it shortener-db psql -U postgres`):

```sql
CREATE TABLE urls (
    code       VARCHAR(8) PRIMARY KEY,
    long_url   TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    clicks     BIGINT NOT NULL DEFAULT 0
);
```

**3. Run and test:**

```bash
go run .

curl -X POST localhost:8080/shorten -d '{"url":"https://google.com"}'
# {"short_url":"http://localhost:8080/AUCG6JWI"}

curl -i localhost:8080/AUCG6JWI
# HTTP/1.1 302 Found
# Location: https://google.com
```

---

## API

| Endpoint | Method | Description |
|---|---|---|
| `/shorten` | POST | Body: `{"url": "https://..."}`. Returns `{"short_url": "..."}`. `400` on invalid or empty URL. |
| `/{code}` | GET | `302` redirect to the original URL. `404` if the code doesn't exist. |
