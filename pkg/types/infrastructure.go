package types

import "time"

// InfrastructureHealth contains all infrastructure health metrics.
type InfrastructureHealth struct {
	Timestamp       time.Time          `json:"timestamp"`
	ControlPlane    ControlPlaneHealth `json:"control_plane"`
	Database        DatabaseHealth     `json:"database"`
	Buffer          BufferHealth       `json:"buffer"`
	StorageForecast StorageForecast    `json:"storage_forecast"`
}

// ControlPlaneHealth contains control plane runtime metrics.
type ControlPlaneHealth struct {
	Status        string  `json:"status"` // healthy, degraded, down
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryMB      float64 `json:"memory_mb"`
	MemoryPercent float64 `json:"memory_percent"`
	Goroutines    int     `json:"goroutines"`
	UptimeSeconds int64   `json:"uptime_seconds"`
}

// DatabaseHealth contains database connection and performance metrics.
type DatabaseHealth struct {
	Status           string       `json:"status"`
	Pool             PoolStats    `json:"pool"`
	SizeBytes        int64        `json:"size_bytes"`
	SizeFormatted    string       `json:"size_formatted"`
	Tables           []TableStats `json:"tables"`
	CompressionRatio float64      `json:"compression_ratio"`
}

// PoolStats contains pgxpool connection pool statistics.
type PoolStats struct {
	TotalConnections    int32 `json:"total_connections"`
	IdleConnections     int32 `json:"idle_connections"`
	AcquiredConnections int32 `json:"acquired_connections"`
	MaxConnections      int32 `json:"max_connections"`
}

// TableStats contains per-table statistics.
type TableStats struct {
	Name             string  `json:"name"`
	SizeBytes        int64   `json:"size_bytes"`
	SizeFormatted    string  `json:"size_formatted"`
	CompressionRatio float64 `json:"compression_ratio,omitempty"`
}

// BufferHealth contains Redis buffer metrics.
type BufferHealth struct {
	Enabled    bool    `json:"enabled"`
	Connected  bool    `json:"connected"`
	QueueDepth int64   `json:"queue_depth"`
	FlushRate  float64 `json:"flush_rate_per_second"`
}

// StorageForecast contains storage growth projections.
type StorageForecast struct {
	DailyGrowthBytes       int64           `json:"daily_growth_bytes"`
	DailyGrowthFormatted   string          `json:"daily_growth_formatted"`
	Projected30dBytes      int64           `json:"projected_30d_bytes"`
	Projected30dFormatted  string          `json:"projected_30d_formatted"`
	DaysUntil100GB         int             `json:"days_until_100gb"`
	RetentionPolicy        RetentionPolicy `json:"retention_policy"`
}

// RetentionPolicy describes data retention settings.
type RetentionPolicy struct {
	RawDataDays         int `json:"raw_data_days"`
	HourlyAggregateDays int `json:"hourly_aggregate_days"`
	DailyAggregateDays  int `json:"daily_aggregate_days"`
}

// BufferStats represents buffer statistics for health reporting.
type BufferStats struct {
	QueueDepth int64
	FlushRate  float64
	Connected  bool
}
