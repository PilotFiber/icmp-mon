// Package cache provides Redis-backed caching for API responses.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// Cache key prefixes
	keyPrefix = "icmpmon:cache:"
)

// Cache provides Redis-backed response caching.
type Cache struct {
	client *redis.Client
	logger *slog.Logger
}

// New creates a new Redis-backed cache.
func New(redisURL string, logger *slog.Logger) (*Cache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &Cache{
		client: client,
		logger: logger,
	}, nil
}

// Get retrieves a cached value. Returns nil if not found or expired.
func (c *Cache) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := c.client.Get(ctx, keyPrefix+key).Bytes()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Set stores a value in the cache with the given TTL.
func (c *Cache) Set(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	return c.client.Set(ctx, keyPrefix+key, data, ttl).Err()
}

// GetJSON retrieves and unmarshals a cached JSON value.
func (c *Cache) GetJSON(ctx context.Context, key string, v any) (bool, error) {
	data, err := c.Get(ctx, key)
	if err != nil {
		return false, err
	}
	if data == nil {
		return false, nil // Cache miss
	}
	if err := json.Unmarshal(data, v); err != nil {
		return false, err
	}
	return true, nil
}

// SetJSON marshals and stores a JSON value in the cache.
func (c *Cache) SetJSON(ctx context.Context, key string, v any, ttl time.Duration) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, data, ttl)
}

// Delete removes a key from the cache.
func (c *Cache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, keyPrefix+key).Err()
}

// DeletePattern removes all keys matching a pattern.
func (c *Cache) DeletePattern(ctx context.Context, pattern string) error {
	keys, err := c.client.Keys(ctx, keyPrefix+pattern).Result()
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		return c.client.Del(ctx, keys...).Err()
	}
	return nil
}
