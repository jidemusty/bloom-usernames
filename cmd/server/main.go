package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jidemusty/bloom-usernames/bloom"
	"github.com/jidemusty/bloom-usernames/checker"
	"github.com/jidemusty/bloom-usernames/username"
	"github.com/redis/go-redis/v9"
)

type server struct {
	pool    *pgxpool.Pool
	checker checker.Checker
}

type checkResponse struct {
	Username  string `json:"username"`
	Available bool   `json:"available"`
	Source    string `json:"source"`
	Micros    int64  `json:"micros"`
}

type signupRequest struct {
	Username string `json:"username"`
}

type signupResponse struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func loadFilter(ctx context.Context, pool *pgxpool.Pool) (*bloom.Filter, error) {
	f := bloom.NewFilterFor(100_000, 0.01)

	rows, err := pool.Query(ctx, "SELECT username FROM users")
	if err != nil {
		return nil, fmt.Errorf("query usernames: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("errors during loop: %w", err)
		}

		f.Add(name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("connection dropped: %w", err)
	}

	return f, nil
}

// loadRedis rebuilds the shared RedisBloom filter from Postgres.
// It builds under a temporary key, then RENAMEs it over the live key in one
// atomic step, so other servers never see an empty or half-built filter.
func loadRedis(ctx context.Context, pool *pgxpool.Pool, rdb *redis.Client, key string) (int, error) {
	tmp := key + ":building"

	// Remove a leftover from a crashed run, then create a fresh filter.
	if err := rdb.Del(ctx, tmp).Err(); err != nil {
		return 0, fmt.Errorf("delete %s: %w", tmp, err)
	}

	if err := rdb.BFReserve(ctx, tmp, 0.01, 100_000).Err(); err != nil {
		return 0, fmt.Errorf("reserve %s: %w", tmp, err)
	}

	rows, err := pool.Query(ctx, "SELECT username FROM users")
	if err != nil {
		return 0, fmt.Errorf("query usernames: %w", err)
	}
	defer rows.Close()

	const batchSize = 1000
	batch := make([]any, 0, batchSize)
	total := 0

	// flush sends the current batch with one BF.MADD (one round trip),
	// then empties the batch while keeping its memory for reuse.
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := rdb.BFMAdd(ctx, tmp, batch...).Err(); err != nil {
			return fmt.Errorf("madd: %w", err)
		}
		total += len(batch)
		batch = batch[:0]
		return nil
	}

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return 0, fmt.Errorf("scan username: %w", err)
		}

		batch = append(batch, name)
		if len(batch) == batchSize {
			if err := flush(); err != nil {
				return 0, err
			}
		}
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("read usernames: %w", err)
	}

	// The last batch is usually partial (e.g. 537 of 100,537 users).
	// Forgetting it would leave names out of the filter: false negatives.
	if err := flush(); err != nil {
		return 0, err
	}

	if err := rdb.Rename(ctx, tmp, key).Err(); err != nil {
		return 0, fmt.Errorf("rename %s to %s: %w", tmp, key, err)
	}

	return total, nil
}

func (s *server) handleSignup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024) // at most 1KB; more become a decode error

	var req signupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cleanedName, err := username.Normalize(req.Username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	displayName := strings.TrimSpace(req.Username)

	// add name to filter
	if err := s.checker.Add(r.Context(), cleanedName); err != nil {
		log.Printf("checker add %q: %v", cleanedName, err)
		http.Error(w, "please try again", http.StatusServiceUnavailable)
		return
	}

	_, err = s.pool.Exec(r.Context(),
		"INSERT INTO users (username, display_name) VALUES ($1, $2)",
		cleanedName, displayName)

	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			http.Error(w, "username taken", http.StatusConflict)
			return
		}

		log.Printf("signup %q: %v", cleanedName, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, signupResponse{
		Username:    cleanedName,
		DisplayName: displayName,
	})
}

func (s *server) handleCheck(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	raw := r.URL.Query().Get("u")

	cleanedName, err := username.Normalize(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	maybe, err := s.checker.MightContain(r.Context(), cleanedName)
	if err != nil {
		log.Printf("checker %s: %v", s.checker.Name(), err)
		maybe = true
	}

	if !maybe {
		writeJSON(w, http.StatusOK, checkResponse{
			Username:  cleanedName,
			Available: true,
			Source:    s.checker.Name(),
			Micros:    time.Since(start).Microseconds(),
		})

		return
	}

	var taken bool
	err = s.pool.QueryRow(r.Context(), "SELECT EXISTS (SELECT 1 FROM users WHERE username = $1)", cleanedName).Scan(&taken)
	if err != nil {
		log.Printf("check %q: %v", cleanedName, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, checkResponse{
		Username:  cleanedName,
		Available: !taken,
		Source:    "db",
		Micros:    time.Since(start).Microseconds(),
	})
}

func main() {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, "postgres://localhost:5432/bloom_usernames")
	if err != nil {
		log.Fatalf("Unable to connect to database pool: %v", err)
	}

	defer pool.Close()

	// ensure the pool can actually reach the db
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Database ping failed: %v", err)
	}

	var c checker.Checker

	switch os.Getenv("CHECKER") {
	case "none":
		c = checker.None{}
		log.Println("CHECKER=none: database only")
	case "", "memory":
		start := time.Now()
		f, err := loadFilter(ctx, pool)
		if err != nil {
			log.Fatalf("load filter %v", err)
		}
		c = checker.NewMemory(f)
		log.Printf("filter loaded in %v", time.Since(start))
	case "redis":
		rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
		defer rdb.Close()

		if err := rdb.Ping(ctx).Err(); err != nil {
			log.Fatalf("redis ping failed: %v", err)
		}

		const key = "users:bloom"
		start := time.Now()
		n, err := loadRedis(ctx, pool, rdb, key)
		if err != nil {
			log.Fatalf("load redis filter: %v", err)
		}
		c = checker.NewRedis(rdb, key)
		log.Printf("redis filter %q loaded with %d names in %v", key, n, time.Since(start))
	default:
		log.Fatalf("unknown CHECKER %q (want none, memory, redis)", os.Getenv("CHECKER"))
	}

	s := &server{pool: pool, checker: c}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/check", s.handleCheck)
	mux.HandleFunc("POST /api/signup", s.handleSignup)
	mux.Handle("GET /", http.FileServer(http.Dir("web")))

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
