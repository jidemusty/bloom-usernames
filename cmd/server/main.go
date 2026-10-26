package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jidemusty/bloom-usernames/bloom"
	"github.com/jidemusty/bloom-usernames/username"
)

type server struct {
	pool   *pgxpool.Pool
	filter *bloom.Filter
}

type checkResponse struct {
	Username  string `json:"username"`
	Available bool   `json:"available"`
	Source    string `json:"source"`
	Micros    int64  `json:"micros"`
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
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

func (s *server) handleCheck(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	raw := r.URL.Query().Get("u")

	cleanedName, err := username.Normalize(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.filter != nil && !s.filter.MightContain(cleanedName) {
		writeJSON(w, checkResponse{
			Username:  cleanedName,
			Available: true,
			Source:    "filter",
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

	writeJSON(w, checkResponse{ // ← replaces the Header().Set + json.NewEncoder lines
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

	var f *bloom.Filter

	if os.Getenv("CHECKER") != "none" {
		start := time.Now()
		f, err = loadFilter(ctx, pool)
		if err != nil {
			log.Fatalf("load filter %v", err)
		}
		log.Printf("filter loaded in %v", time.Since(start))
	} else {
		log.Println("CHECKER=none: database only")
	}

	s := &server{pool: pool, filter: f}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/check", s.handleCheck)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
	// if err := http.ListenAndServe(":8080", mux); err != nil && err != http.ErrServerClosed {
	// 	log.Fatalf("Server forced to shutdown: %v\n", err)
	// }
}
