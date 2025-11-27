// Package buffer provides a Redis-backed write-ahead buffer for probe results.
// This decouples agent result ingestion from database writes, allowing for
// much higher throughput and resilience to database slowdowns.
package buffer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/pilot-net/icmp-mon/pkg/types"
)

const (
	// Redis key for the probe results queue
	keyProbeResults = "icmpmon:probe_results"

	// Default batch size for flushing - COPY handles large batches efficiently
	DefaultBatchSize = 20000

	// Default flush interval - flush more frequently to keep up with probe volume
	DefaultFlushInterval = 2 * time.Second
)

// ResultBuffer provides Redis-backed buffering for probe results.
type ResultBuffer struct {
	client *redis.Client
	logger *slog.Logger
}

// NewResultBuffer creates a new Redis-backed result buffer.
func NewResultBuffer(redisURL string, logger *slog.Logger) (*ResultBuffer, error) {
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

	return &ResultBuffer{
		client: client,
		logger: logger,
	}, nil
}

// Push adds probe results to the buffer.
// Results are JSON-encoded and pushed to a Redis list.
func (b *ResultBuffer) Push(ctx context.Context, results []types.ProbeResult) error {
	if len(results) == 0 {
		return nil
	}

	// Serialize each result to JSON
	values := make([]interface{}, len(results))
	for i, r := range results {
		data, err := json.Marshal(r)
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
		values[i] = data
	}

	// Push all results atomically
	if err := b.client.LPush(ctx, keyProbeResults, values...).Err(); err != nil {
		return fmt.Errorf("failed to push results to redis: %w", err)
	}

	return nil
}

// Pop retrieves and removes up to maxResults from the buffer.
// Returns the results in FIFO order.
func (b *ResultBuffer) Pop(ctx context.Context, maxResults int) ([]types.ProbeResult, error) {
	// Use RPOP to get oldest items first (FIFO)
	pipe := b.client.Pipeline()
	cmds := make([]*redis.StringCmd, maxResults)

	for i := 0; i < maxResults; i++ {
		cmds[i] = pipe.RPop(ctx, keyProbeResults)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to pop results from redis: %w", err)
	}

	results := make([]types.ProbeResult, 0, maxResults)
	for _, cmd := range cmds {
		data, err := cmd.Bytes()
		if err == redis.Nil {
			continue // No more items
		}
		if err != nil {
			continue // Skip errors for individual items
		}

		var r types.ProbeResult
		if err := json.Unmarshal(data, &r); err != nil {
			b.logger.Warn("failed to unmarshal probe result", "error", err)
			continue
		}
		results = append(results, r)
	}

	return results, nil
}

// Len returns the number of buffered results.
func (b *ResultBuffer) Len(ctx context.Context) (int64, error) {
	return b.client.LLen(ctx, keyProbeResults).Result()
}

// Close closes the Redis connection.
func (b *ResultBuffer) Close() error {
	return b.client.Close()
}
