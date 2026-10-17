package cli

import (
	"context"
	"log/slog"
	"time"

	"github.com/biliblihuorong/imgnest/internal/cache"
	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/randompool"
	"github.com/biliblihuorong/imgnest/internal/service"
)

// lookupCacheEntries bounds the in-process variant lookup cache.
const lookupCacheEntries = 16384

// servingCaches are the random-link pool and the variant lookup cache.
type servingCaches struct {
	pool   service.RandomPool
	lookup service.ByteCache
}

// openCaches uses Redis when cache.redis_url is set and reachable, and this
// process's memory otherwise. An unreachable Redis at startup is logged and
// the server keeps running on memory, the same as having none configured;
// later Redis outages only cost each request a database read.
func openCaches(ctx context.Context, cfg config.Cache, logger *slog.Logger) (servingCaches, func(), error) {
	memory := servingCaches{pool: randompool.NewMemory(time.Now), lookup: cache.NewMemory(lookupCacheEntries, time.Now)}
	if cfg.RedisURL == "" {
		logger.Info("cache backend", "backend", "memory")
		return memory, func() {}, nil
	}
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	redis, err := cache.OpenRedis(dialCtx, cfg.RedisURL)
	if err != nil {
		if ctx.Err() != nil {
			return servingCaches{}, nil, ctx.Err()
		}
		// The error never carries the URL, which may hold a password.
		logger.Warn("redis unavailable, caching in memory", "error", err.Error())
		return memory, func() {}, nil
	}
	logger.Info("cache backend", "backend", "redis")
	closeRedis := func() {
		if err := redis.Close(); err != nil {
			logger.Error("close redis", "error", err.Error())
		}
	}
	return servingCaches{pool: randompool.NewShared(redis), lookup: redis}, closeRedis, nil
}
