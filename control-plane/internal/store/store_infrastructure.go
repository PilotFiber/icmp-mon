package store

import (
	"context"
	"fmt"

	"github.com/pilot-net/icmp-mon/pkg/types"
)

// GetDatabaseSize returns the total size of the database in bytes.
func (s *Store) GetDatabaseSize(ctx context.Context) (int64, error) {
	var size int64
	err := s.pool.QueryRow(ctx, `
		SELECT pg_database_size(current_database())
	`).Scan(&size)
	return size, err
}

// GetTableStats returns size and compression statistics for all hypertables.
func (s *Store) GetTableStats(ctx context.Context) ([]types.TableStats, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			h.hypertable_name::text,
			hypertable_size(format('%I.%I', h.hypertable_schema, h.hypertable_name)::regclass)::bigint as size_bytes,
			COALESCE(
				(SELECT
					CASE
						WHEN cs.after_compression_total_bytes > 0
						THEN cs.before_compression_total_bytes::float / cs.after_compression_total_bytes::float
						ELSE 0
					END
				FROM hypertable_compression_stats(format('%I.%I', h.hypertable_schema, h.hypertable_name)::regclass) cs
				WHERE cs.after_compression_total_bytes IS NOT NULL),
				0
			)::float as compression_ratio
		FROM timescaledb_information.hypertables h
		ORDER BY hypertable_size(format('%I.%I', h.hypertable_schema, h.hypertable_name)::regclass) DESC NULLS LAST
	`)
	if err != nil {
		return nil, fmt.Errorf("querying table stats: %w", err)
	}
	defer rows.Close()

	var stats []types.TableStats
	for rows.Next() {
		var ts types.TableStats
		if err := rows.Scan(&ts.Name, &ts.SizeBytes, &ts.CompressionRatio); err != nil {
			return nil, fmt.Errorf("scanning table stats: %w", err)
		}
		ts.SizeFormatted = formatBytes(ts.SizeBytes)
		stats = append(stats, ts)
	}
	return stats, rows.Err()
}

// GetCompressionRatio returns the overall compression ratio for the database.
// Returns 0 if no compression has been applied yet.
func (s *Store) GetCompressionRatio(ctx context.Context) (float64, error) {
	var ratio float64
	// Sum compression stats across all hypertables
	err := s.pool.QueryRow(ctx, `
		WITH compression_totals AS (
			SELECT
				SUM(cs.before_compression_total_bytes) as before_total,
				SUM(cs.after_compression_total_bytes) as after_total
			FROM timescaledb_information.hypertables h
			CROSS JOIN LATERAL hypertable_compression_stats(format('%I.%I', h.hypertable_schema, h.hypertable_name)::regclass) cs
			WHERE cs.after_compression_total_bytes IS NOT NULL AND cs.after_compression_total_bytes > 0
		)
		SELECT COALESCE(
			CASE WHEN after_total > 0 THEN before_total::float / after_total::float ELSE 0 END,
			0
		)
		FROM compression_totals
	`).Scan(&ratio)
	return ratio, err
}

// GetDailyGrowthBytes calculates the average daily growth rate based on probe_results data age and size.
func (s *Store) GetDailyGrowthBytes(ctx context.Context) (int64, error) {
	var growth int64
	// Estimate based on probe_results table age and size
	err := s.pool.QueryRow(ctx, `
		WITH table_info AS (
			SELECT
				hypertable_size(format('%I.%I', h.hypertable_schema, h.hypertable_name)::regclass) as size_bytes
			FROM timescaledb_information.hypertables h
			WHERE h.hypertable_name = 'probe_results'
		),
		age_info AS (
			SELECT GREATEST(EXTRACT(EPOCH FROM (NOW() - MIN(time))) / 86400, 1) as days_of_data
			FROM probe_results
		)
		SELECT COALESCE((SELECT size_bytes FROM table_info) / (SELECT days_of_data FROM age_info), 0)::bigint
	`).Scan(&growth)

	return growth, err
}

// GetStorageForecast returns storage growth projections.
func (s *Store) GetStorageForecast(ctx context.Context) (*types.StorageForecast, error) {
	currentSize, err := s.GetDatabaseSize(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting database size: %w", err)
	}

	dailyGrowth, err := s.GetDailyGrowthBytes(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting daily growth: %w", err)
	}

	forecast := &types.StorageForecast{
		DailyGrowthBytes:      dailyGrowth,
		DailyGrowthFormatted:  formatBytes(dailyGrowth),
		Projected30dBytes:     currentSize + (dailyGrowth * 30),
		Projected30dFormatted: formatBytes(currentSize + (dailyGrowth * 30)),
		RetentionPolicy: types.RetentionPolicy{
			RawDataDays:         90,
			HourlyAggregateDays: 365,
			DailyAggregateDays:  730,
		},
	}

	// Calculate days until 100GB
	const targetSize int64 = 100 * 1024 * 1024 * 1024 // 100 GB
	if currentSize >= targetSize {
		forecast.DaysUntil100GB = 0
	} else if dailyGrowth > 0 {
		forecast.DaysUntil100GB = int((targetSize - currentSize) / dailyGrowth)
	} else {
		forecast.DaysUntil100GB = -1 // Indicates unknown/not growing
	}

	return forecast, nil
}

// GetPoolStats returns the current connection pool statistics.
func (s *Store) GetPoolStats() types.PoolStats {
	stat := s.pool.Stat()
	return types.PoolStats{
		TotalConnections:    stat.TotalConns(),
		IdleConnections:     stat.IdleConns(),
		AcquiredConnections: stat.AcquiredConns(),
		MaxConnections:      stat.MaxConns(),
	}
}

// RecordDatabaseSizes records current table sizes to the history table.
// This should be called periodically (e.g., daily) by a background job.
func (s *Store) RecordDatabaseSizes(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT record_database_sizes()`).Scan(&count)
	return count, err
}

// formatBytes converts bytes to a human-readable string.
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
