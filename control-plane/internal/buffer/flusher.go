package buffer

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pilot-net/icmp-mon/pkg/types"
)

// Flusher reads from the Redis buffer and writes to TimescaleDB.
type Flusher struct {
	buffer   *ResultBuffer
	pool     *pgxpool.Pool
	logger   *slog.Logger
	interval time.Duration
	batch    int

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewFlusher creates a new buffer flusher.
func NewFlusher(buffer *ResultBuffer, pool *pgxpool.Pool, logger *slog.Logger) *Flusher {
	return &Flusher{
		buffer:   buffer,
		pool:     pool,
		logger:   logger.With("component", "buffer_flusher"),
		interval: DefaultFlushInterval,
		batch:    DefaultBatchSize,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the background flushing loop.
func (f *Flusher) Start() {
	f.wg.Add(1)
	go f.run()
	f.logger.Info("buffer flusher started", "interval", f.interval, "batch_size", f.batch)
}

// Stop stops the flusher and waits for completion.
func (f *Flusher) Stop() {
	close(f.stopCh)
	f.wg.Wait()
	f.logger.Info("buffer flusher stopped")
}

func (f *Flusher) run() {
	defer f.wg.Done()

	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()

	for {
		select {
		case <-f.stopCh:
			// Final flush before stopping
			f.flush()
			return
		case <-ticker.C:
			f.flush()
		}
	}
}

func (f *Flusher) flush() {
	ctx := context.Background()

	// Check buffer size
	size, err := f.buffer.Len(ctx)
	if err != nil {
		f.logger.Error("failed to get buffer size", "error", err)
		return
	}

	if size == 0 {
		return
	}

	// Pop results from buffer
	results, err := f.buffer.Pop(ctx, f.batch)
	if err != nil {
		f.logger.Error("failed to pop from buffer", "error", err)
		return
	}

	if len(results) == 0 {
		return
	}

	start := time.Now()

	// Use COPY for maximum throughput
	err = f.copyResults(ctx, results)
	if err != nil {
		f.logger.Error("failed to copy results to database",
			"error", err,
			"count", len(results),
		)
		// TODO: Consider pushing failed results back to buffer or a dead-letter queue
		return
	}

	f.logger.Info("flushed results to database",
		"count", len(results),
		"remaining", size-int64(len(results)),
		"duration", time.Since(start),
	)
}

// copyResults uses PostgreSQL COPY via a temp table for high-throughput bulk inserts.
// This approach allows handling duplicates gracefully (ON CONFLICT DO NOTHING).
func (f *Flusher) copyResults(ctx context.Context, results []types.ProbeResult) error {
	// Use a transaction to ensure temp table cleanup
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Create temp table matching probe_results structure
	_, err = tx.Exec(ctx, `
		CREATE TEMP TABLE probe_results_staging (
			time TIMESTAMPTZ NOT NULL,
			target_id UUID NOT NULL,
			agent_id UUID NOT NULL,
			success BOOLEAN NOT NULL,
			error_message TEXT,
			latency_ms DOUBLE PRECISION,
			packet_loss_pct DOUBLE PRECISION,
			payload JSONB
		) ON COMMIT DROP
	`)
	if err != nil {
		return err
	}

	// COPY data into temp table (very fast)
	rows := make([][]any, len(results))
	for i, r := range results {
		rows[i] = []any{
			r.Timestamp, r.TargetID, r.AgentID, r.Success, r.Error,
			getLatency(r.Payload), getPacketLoss(r.Payload), r.Payload,
		}
	}

	_, err = tx.CopyFrom(ctx,
		pgx.Identifier{"probe_results_staging"},
		[]string{"time", "target_id", "agent_id", "success", "error_message", "latency_ms", "packet_loss_pct", "payload"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return err
	}

	// INSERT from temp to permanent table with conflict handling
	// Computes agent_region, target_region, and is_in_market via JOINs
	// Gateway targets are excluded from region metrics (they deprioritize ICMP, skewing latency)
	_, err = tx.Exec(ctx, `
		INSERT INTO probe_results (time, target_id, agent_id, success, error_message, latency_ms, packet_loss_pct, payload,
		                           agent_region, target_region, is_in_market)
		SELECT
			s.time, s.target_id, s.agent_id, s.success, s.error_message, s.latency_ms, s.packet_loss_pct, s.payload,
			CASE WHEN t.ip_type = 'gateway' THEN NULL ELSE LOWER(TRIM(a.region)) END,
			CASE WHEN t.ip_type = 'gateway' THEN NULL ELSE LOWER(TRIM(sub.region)) END,
			CASE WHEN t.ip_type = 'gateway' THEN NULL ELSE
				(LOWER(TRIM(COALESCE(a.region, ''))) = LOWER(TRIM(COALESCE(sub.region, '')))
				 AND a.region IS NOT NULL AND a.region != ''
				 AND sub.region IS NOT NULL AND sub.region != '')
			END
		FROM probe_results_staging s
		JOIN agents a ON s.agent_id = a.id
		LEFT JOIN targets t ON s.target_id = t.id
		LEFT JOIN subnets sub ON t.subnet_id = sub.id
		ON CONFLICT (time, target_id, agent_id) DO NOTHING
	`)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// getLatency extracts avg_ms from payload if present
func getLatency(payload json.RawMessage) *float64 {
	var p struct {
		AvgMs float64 `json:"avg_ms"`
	}
	if err := json.Unmarshal(payload, &p); err == nil && p.AvgMs > 0 {
		return &p.AvgMs
	}
	return nil
}

// getPacketLoss extracts packet_loss_pct from payload if present
func getPacketLoss(payload json.RawMessage) *float64 {
	var p struct {
		PacketLoss float64 `json:"packet_loss_pct"`
	}
	if err := json.Unmarshal(payload, &p); err == nil {
		return &p.PacketLoss
	}
	return nil
}
