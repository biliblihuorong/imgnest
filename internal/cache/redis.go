package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisKeyPrefix namespaces every key so one Redis can serve other apps.
const redisKeyPrefix = "imgnest:"

// Redis is the Store shared by every instance pointed at the same server.
type Redis struct {
	client *redis.Client
}

// errRedisURL hides the URL, which may carry a password, from errors.
var errRedisURL = errors.New("invalid redis url")

// OpenRedis connects to a redis:// or rediss:// URL and checks it answers.
// Timeouts the URL leaves unset are kept short: a slow cache must cost a
// request little more than the database read it was meant to save.
func OpenRedis(ctx context.Context, rawURL string) (*Redis, error) {
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, errRedisURL
	}
	if options.DialTimeout == 0 {
		options.DialTimeout = 2 * time.Second
	}
	if options.ReadTimeout == 0 {
		options.ReadTimeout = 500 * time.Millisecond
	}
	if options.WriteTimeout == 0 {
		options.WriteTimeout = 500 * time.Millisecond
	}
	options.MaxRetries = 1
	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect: %w", redisError(err))
	}
	return &Redis{client: client}, nil
}

// redisError keeps the failure kind without the server's address.
func redisError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("redis: %w", err)
	default:
		return fmt.Errorf("redis: %w", errUnavailable)
	}
}

// errUnavailable reports a Redis failure without its address or reply text.
var errUnavailable = errors.New("cache unavailable")

// Get reads one value; a missing key is a miss, not an error.
func (r *Redis) Get(ctx context.Context, key string) ([]byte, bool, error) {
	value, err := r.client.Get(ctx, redisKeyPrefix+key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, redisError(err)
	}
	return value, true, nil
}

// Set writes one value with its expiry; a non-positive ttl deletes it.
func (r *Redis) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		return r.Delete(ctx, key)
	}
	if err := r.client.Set(ctx, redisKeyPrefix+key, value, ttl).Err(); err != nil {
		return redisError(err)
	}
	return nil
}

// Delete drops keys; unknown keys are not an error.
func (r *Redis) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	prefixed := make([]string, len(keys))
	for i, key := range keys {
		prefixed[i] = redisKeyPrefix + key
	}
	if err := r.client.Del(ctx, prefixed...).Err(); err != nil {
		return redisError(err)
	}
	return nil
}

// Close releases the connection pool.
func (r *Redis) Close() error {
	return r.client.Close()
}
