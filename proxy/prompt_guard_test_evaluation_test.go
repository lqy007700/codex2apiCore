package proxy

import (
	"context"
	"encoding/json"

	"sync/atomic"

	"time"

	"github.com/codex2api/cache"
)

type promptGuardRuntimeWriteCounter struct {
	cache.TokenCache
	writes atomic.Int64
}

func (c *promptGuardRuntimeWriteCounter) SetRuntime(ctx context.Context, namespace string, key string, value json.RawMessage, ttl time.Duration) error {
	c.writes.Add(1)
	return c.TokenCache.SetRuntime(ctx, namespace, key, value, ttl)
}
