package checker

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis is a Checker backed by a RedisBloom filter shared by every server.
// Redis runs one command at a time, so no mutex is needed on the Go side.
type Redis struct {
	client *redis.Client
	key    string
}

func NewRedis(client *redis.Client, key string) *Redis {
	return &Redis{client: client, key: key}
}

func (r *Redis) MightContain(ctx context.Context, name string) (bool, error) {
	// Give up quickly: an error makes the handler fall back to the database.
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	return r.client.BFExists(ctx, r.key, name).Result()
}

func (r *Redis) Add(ctx context.Context, name string) error {
	// BF.ADD's bool means "were all bits already on?", which can be a false
	// positive, so it must never decide anything. Only the error matters.
	_, err := r.client.BFAdd(ctx, r.key, name).Result()
	return err
}

func (r *Redis) Name() string {
	return "redis"
}
