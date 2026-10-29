# bloom-usernames

Do sites really check username availability with bloom filters? This project answers that by building one end to end.

A Go server checks usernames against Postgres in one of three ways:

| `CHECKER` | How a check works |
|---|---|
| `none` | Ask Postgres every time (the baseline). |
| `memory` | Ask a bloom filter written from scratch, held in the server's memory. Only "maybe taken" goes to Postgres. |
| `redis` | Ask a shared RedisBloom filter. Only "maybe taken" goes to Postgres. |

A small page checks names live as you type and shows which layer answered and how long it took. Under k6 load, both filters cut database queries by about 90%, at a measured false-positive rate of 0.5–1.2%.

The surprise: a well-indexed Postgres handled about 37,000 checks per second on its own. A bloom filter protects the database; it doesn't make one check feel faster.

## How a check flows

```
name ─▶ normalize ─▶ checker.MightContain?
                        │ no  ─▶ "available"   (Postgres never asked)
                        │ yes ─▶ SELECT EXISTS ─▶ "available" / "taken"
```

- **A bloom filter can prove a name is free, never that it's taken.** "No" is always correct. "Maybe" goes to Postgres.
- **Signup never checks first.** It adds the name to the filter, then runs `INSERT`. The `UNIQUE` constraint picks one winner when two people sign up for the same name at once, and the other gets `409`. In a test with 50 parallel signups for one name, exactly one succeeded.
- **The filter is updated before the insert.** If the insert then fails, the name is just an extra false positive, which is harmless. The other order would briefly report a taken name as free.
- **Failure handling.** If the checker errors, a check asks Postgres instead (it fails open), but a signup returns `503` (it fails closed), so the filter never misses a stored name.

## Results

50 virtual users for 30 s, with 90% free names and 10% taken names (`loadtest/check.js`). Everything ran on one laptop, so the numbers are good for comparing the modes, not as production figures.

| Mode | req/s | Median | p95 | Checks hitting Postgres | False-positive rate |
|---|---|---|---|---|---|
| none | 36,953 | 1.05 ms | 1.66 ms | 100% | — |
| memory | 46,856 | 78 µs | 5.05 ms | 11.1% | 1.17% |
| redis | 40,895 | 977 µs | 2.51 ms | 10.4% | 0.50% |

- About 10% of checks still reach Postgres because taken names always need the database. Another ~1% are false positives.
- Over a localhost connection, a Redis round trip costs about the same as a Postgres one. Redis is there to give every server one shared filter that survives restarts, not to make checks faster.
- RedisBloom gives its filter more bits than the textbook formula (≈135 KB vs ≈117 KB for 100,000 names at 1%), which is why its false-positive rate came in lower.

## What's inside

| Path | What it is |
|---|---|
| `bloom/` | The filter itself: sizing math (`m = −n·ln p / (ln 2)²`, `k = (m/n)·ln 2`), bits packed into bytes, FNV-1a with double hashing, and a `sync.RWMutex` (many readers at once, one writer). Tested for zero false negatives, the false-positive rate and data races. |
| `username/` | One `Normalize` function (trim, lowercase, `^[a-z0-9_]{3,20}$`). The database `CHECK` constraint enforces the same rule. |
| `checker/` | A `Checker` interface with `None`, `Memory` and `Redis` implementations. |
| `cmd/seed/` | Loads 100,000 users (`user0` to `user99999`) with Postgres `COPY`, about 26× faster than one `INSERT` per row. |
| `cmd/server/` | HTTP API, startup filter loading, and atomic counters for `/api/stats`. The Redis filter is rebuilt under a temporary key and swapped in with `RENAME`, so other servers never see a half-built filter. |
| `web/` | The live-check page. It waits for a 250 ms pause in typing and cancels outdated requests with `AbortController`. |
| `loadtest/` | k6 script. |

## Run it

You need Go 1.25+, Postgres, and Redis 8+ (bloom filter commands built in) or Redis Stack.

```sh
createdb bloom_usernames
psql bloom_usernames -f db/schema.sql
go run ./cmd/seed

CHECKER=memory go run ./cmd/server   # or none, or redis
open http://localhost:8080
```

Run the commands from the repository root, because the server serves files from `web/` relative to where it starts.

| Endpoint | |
|---|---|
| `GET /api/check?u=Name` | `{"username","available","source","micros"}` |
| `POST /api/signup` `{"username":"Name"}` | `201` created, `409` taken, `400` invalid, `503` filter unavailable |
| `GET /api/stats` | counts of checks, checker answers, database queries and false positives |

```sh
go test -race ./...
k6 run loadtest/check.js && curl -s localhost:8080/api/stats
```

## Write-up

The story behind the project, and what the numbers mean: [jidemusty.github.io/concepts/bloom-filters](https://jidemusty.github.io/concepts/bloom-filters/)
