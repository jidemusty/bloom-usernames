package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jidemusty/bloom-usernames/username"
)

type server struct {
	pool *pgxpool.Pool
}

type checkResponse struct {
	Username  string `json:"username"`
	Available bool   `json:"available"`
	Source    string `json:"source"`
	Micros    int64  `json:"micros"`
}

func (s *server) handleCheck(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	raw := r.URL.Query().Get("u")

	cleanedName, err := username.Normalize(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var taken bool
	err = s.pool.QueryRow(r.Context(), "SELECT EXISTS (SELECT 1 FROM users WHERE username = $1)", cleanedName).Scan(&taken)
	if err != nil {
		log.Printf("check %q: %v", cleanedName, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(checkResponse{
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
		log.Fatalf("Database ping failed: %v\n", err)
	}

	s := &server{pool: pool}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/check", s.handleCheck)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
	// if err := http.ListenAndServe(":8080", mux); err != nil && err != http.ErrServerClosed {
	// 	log.Fatalf("Server forced to shutdown: %v\n", err)
	// }
}
