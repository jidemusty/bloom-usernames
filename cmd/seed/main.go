package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, "postgres://localhost:5432/bloom_usernames")
	if err != nil {
		log.Fatalf("connect: %v", err)
	}

	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, "TRUNCATE users RESTART IDENTITY")
	if err != nil {
		log.Fatalf("truncate: %v", err)
	}

	start := time.Now()

	rows := make([][]any, 0, 100_000)
	for i := range 100_000 {
		name := fmt.Sprintf("user%d", i)
		rows = append(rows, []any{name, name})
	}

	count, err := conn.CopyFrom(
		ctx,
		pgx.Identifier{"users"},
		[]string{"username", "display_name"},
		pgx.CopyFromRows(rows),
	)

	if err != nil {
		log.Fatalf("copyfrom: %v", err)
	}

	fmt.Printf("inserted %d rows in %v\n", count, time.Since(start))
}
