# bloom-usernames

Do username checks need a Bloom filter? This project compares three ways to answer that question with a Go server, Postgres, and RedisBloom.

Set `CHECKER` to choose the lookup path:

| `CHECKER` | Lookup path |
|---|---|
| `none` | Query Postgres for every check. |
| `memory` | Check a Bloom filter in the server. Send possible matches to Postgres. |
| `redis` | Check a shared RedisBloom filter. Send possible matches to Postgres. |

The browser checks a name as you type and reports which layer answered and how long it took. In a local k6 run, the filters reduced Postgres queries by about 90%. The measured false-positive rate was 0.5–1.2%.

Postgres alone handled about 37,000 checks per second with an index. The filter reduced database traffic, but it did not make an individual check feel faster.

## How checks work

```text
name ─▶ normalize ─▶ checker.MightContain?
                        │ no  ─▶ available (no Postgres query)
                        │ yes ─▶ SELECT EXISTS ─▶ available / taken
```

A Bloom filter can tell us that a name is free. If it says a name might be present, Postgres confirms whether it is taken.

Signup follows a different path. It adds the normalized name to the filter, then tries the database insert. Postgres's `UNIQUE` constraint decides the winner if two signups race; the other request gets `409`. In a run with 50 simultaneous signups for one name, one succeeded.

Adding to the filter first can leave an extra entry if the insert fails. That only sends a later check to Postgres. Inserting first could briefly tell a check that a newly stored name is free.

If a checker errors, availability checks fall back to Postgres. Signup returns `503` instead, so it cannot add a name without updating the filter.

## Benchmark

The run used 50 virtual users for 30 seconds, with 90% free names and 10% taken names. The script is `loadtest/check.js`. These are single-laptop results, useful for comparing the modes rather than predicting production throughput.

| Mode | req/s | Median | p95 | Checks sent to Postgres | False-positive rate |
|---|---:|---:|---:|---:|---:|
| none | 36,953 | 1.05 ms | 1.66 ms | 100% | — |
| memory | 46,856 | 78 µs | 5.05 ms | 11.1% | 1.17% |
| redis | 40,895 | 977 µs | 2.51 ms | 10.4% | 0.50% |

Taken names still require a database query, as do false positives. That accounts for most of the roughly 10% of checks that reached Postgres in the filtered runs.

On localhost, a Redis round trip cost about as much as a Postgres query. Redis's role here is to give multiple server instances one shared filter that survives a server restart.

For 100,000 names at a 1% target false-positive rate, the textbook formula calls for about 117 KB. RedisBloom used about 135 KB and measured a lower false-positive rate.

## Project layout

| Path | Contents |
|---|---|
| `bloom/` | Bloom filter sizing, packed bits, FNV-1a with double hashing, and a `sync.RWMutex`. Tests cover false negatives, false-positive rate, and data races. |
| `username/` | `Normalize`: trim, lowercase, then validate against `^[a-z0-9_]{3,20}$`. The database has a matching `CHECK` constraint. |
| `checker/` | The `Checker` interface and `None`, `Memory`, and `Redis` implementations. |
| `cmd/seed/` | Seeds 100,000 users (`user0` through `user99999`) with Postgres `COPY`, about 26 times faster than inserting rows one at a time. |
| `cmd/server/` | HTTP API, startup filter loading, and `/api/stats` counters. Redis mode rebuilds the filter under a temporary key, then swaps it in with `RENAME`. |
| `web/` | Live username checker. It waits 250 ms after typing and cancels outdated requests with `AbortController`. |
| `loadtest/` | k6 benchmark script. |

## Run locally

You'll need Go 1.25 or newer, Postgres, and Redis 8 or newer with Bloom filter commands enabled (or Redis Stack).

From the repository root:

```sh
createdb bloom_usernames
psql bloom_usernames -f db/schema.sql
go run ./cmd/seed

CHECKER=memory go run ./cmd/server   # also: none or redis
```

Then open <http://localhost:8080>. Run the server from the repository root so it can find `web/`.

| Endpoint | Response |
|---|---|
| `GET /api/check?u=Name` | `username`, `available`, `source`, and `micros` |
| `POST /api/signup` with `{"username":"Name"}` | `201` created, `409` taken, `400` invalid, or `503` filter unavailable |
| `GET /api/stats` | Counts of checks, checker answers, database queries, and false positives |

Run the Go tests and the load test with:

```sh
go test -race ./...
k6 run loadtest/check.js
curl -s localhost:8080/api/stats
```

## Write-up

More detail on the experiment and its results: [Bloom filters](https://jidemusty.github.io/concepts/bloom-filters/).
