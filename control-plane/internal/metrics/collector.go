// Package metrics provides infrastructure metrics collection for the control plane.
package metrics

import (
	"context"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/pilot-net/icmp-mon/control-plane/internal/store"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// BufferStatsProvider is an interface for getting buffer statistics.
type BufferStatsProvider interface {
	GetStats(ctx context.Context) (types.BufferStats, error)
}

// Collector gathers infrastructure metrics with caching.
type Collector struct {
	store  *store.Store
	buffer BufferStatsProvider // may be nil if buffer is disabled

	startTime time.Time

	// Cached values with TTL
	mu            sync.RWMutex
	cachedHealth  *types.InfrastructureHealth
	cacheExpiry   time.Time
	cacheDuration time.Duration
}

// NewCollector creates a new metrics collector.
func NewCollector(store *store.Store, buffer BufferStatsProvider) *Collector {
	return &Collector{
		store:         store,
		buffer:        buffer,
		startTime:     time.Now(),
		cacheDuration: 30 * time.Second,
	}
}

// GetInfrastructureHealth returns the current infrastructure health metrics.
// Results are cached for 30 seconds to avoid expensive database queries.
func (c *Collector) GetInfrastructureHealth(ctx context.Context) (*types.InfrastructureHealth, error) {
	c.mu.RLock()
	if c.cachedHealth != nil && time.Now().Before(c.cacheExpiry) {
		health := *c.cachedHealth
		c.mu.RUnlock()
		return &health, nil
	}
	c.mu.RUnlock()

	// Collect fresh metrics
	health, err := c.collectHealth(ctx)
	if err != nil {
		return nil, err
	}

	// Update cache
	c.mu.Lock()
	c.cachedHealth = health
	c.cacheExpiry = time.Now().Add(c.cacheDuration)
	c.mu.Unlock()

	return health, nil
}

func (c *Collector) collectHealth(ctx context.Context) (*types.InfrastructureHealth, error) {
	health := &types.InfrastructureHealth{
		Timestamp: time.Now(),
	}

	// Collect control plane metrics (always available)
	health.ControlPlane = c.collectControlPlaneHealth()

	// Collect database metrics
	dbHealth, err := c.collectDatabaseHealth(ctx)
	if err != nil {
		health.Database = types.DatabaseHealth{
			Status: "error",
		}
	} else {
		health.Database = *dbHealth
	}

	// Collect buffer metrics if enabled
	health.Buffer = c.collectBufferHealth(ctx)

	// Collect storage forecast
	forecast, err := c.store.GetStorageForecast(ctx)
	if err != nil {
		health.StorageForecast = types.StorageForecast{}
	} else {
		health.StorageForecast = *forecast
	}

	return health, nil
}

func (c *Collector) collectControlPlaneHealth() types.ControlPlaneHealth {
	health := types.ControlPlaneHealth{
		Status:        "healthy",
		Goroutines:    runtime.NumGoroutine(),
		UptimeSeconds: int64(time.Since(c.startTime).Seconds()),
	}

	// Get process metrics using gopsutil
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err == nil {
		// CPU percent (over last second)
		if cpu, err := proc.CPUPercent(); err == nil {
			health.CPUPercent = cpu
		}

		// Memory info
		if mem, err := proc.MemoryInfo(); err == nil {
			health.MemoryMB = float64(mem.RSS) / (1024 * 1024)
		}

		// Memory percent
		if memPct, err := proc.MemoryPercent(); err == nil {
			health.MemoryPercent = float64(memPct)
		}
	}

	// Determine status based on metrics
	if health.MemoryPercent > 90 || health.CPUPercent > 90 {
		health.Status = "degraded"
	}

	return health
}

func (c *Collector) collectDatabaseHealth(ctx context.Context) (*types.DatabaseHealth, error) {
	health := &types.DatabaseHealth{
		Status: "healthy",
	}

	// Get pool stats (always available, no query needed)
	health.Pool = c.store.GetPoolStats()

	// Check pool health
	if health.Pool.AcquiredConnections >= health.Pool.MaxConnections-2 {
		health.Status = "degraded"
	}

	// Get database size
	size, err := c.store.GetDatabaseSize(ctx)
	if err != nil {
		return nil, err
	}
	health.SizeBytes = size
	health.SizeFormatted = formatBytes(size)

	// Get table stats
	tables, err := c.store.GetTableStats(ctx)
	if err != nil {
		// Non-fatal, continue with empty tables
		tables = []types.TableStats{}
	}
	health.Tables = tables

	// Get overall compression ratio
	ratio, err := c.store.GetCompressionRatio(ctx)
	if err == nil {
		health.CompressionRatio = ratio
	}

	return health, nil
}

func (c *Collector) collectBufferHealth(ctx context.Context) types.BufferHealth {
	if c.buffer == nil {
		return types.BufferHealth{
			Enabled:   false,
			Connected: false,
		}
	}

	stats, err := c.buffer.GetStats(ctx)
	if err != nil {
		return types.BufferHealth{
			Enabled:   true,
			Connected: false,
		}
	}

	return types.BufferHealth{
		Enabled:    true,
		Connected:  stats.Connected,
		QueueDepth: stats.QueueDepth,
		FlushRate:  stats.FlushRate,
	}
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
		return formatFloat(float64(bytes)/TB) + " TB"
	case bytes >= GB:
		return formatFloat(float64(bytes)/GB) + " GB"
	case bytes >= MB:
		return formatFloat(float64(bytes)/MB) + " MB"
	case bytes >= KB:
		return formatFloat(float64(bytes)/KB) + " KB"
	default:
		return formatInt(bytes) + " B"
	}
}

func formatFloat(f float64) string {
	if f >= 100 {
		return formatInt(int64(f))
	}
	if f >= 10 {
		return trimTrailingZeros(f, 1)
	}
	return trimTrailingZeros(f, 2)
}

func formatInt(i int64) string {
	s := ""
	if i < 0 {
		s = "-"
		i = -i
	}
	str := ""
	for i > 0 || str == "" {
		if len(str) > 0 && len(str)%3 == 0 {
			str = "," + str
		}
		str = string('0'+byte(i%10)) + str
		i /= 10
	}
	return s + str
}

func trimTrailingZeros(f float64, decimals int) string {
	format := "%." + string('0'+byte(decimals)) + "f"
	s := ""
	switch decimals {
	case 1:
		s = formatWithPrecision(f, 1)
	case 2:
		s = formatWithPrecision(f, 2)
	default:
		s = formatWithPrecision(f, decimals)
	}
	_ = format // unused, keeping for documentation
	return s
}

func formatWithPrecision(f float64, precision int) string {
	// Simple float formatting without fmt package in hot path
	intPart := int64(f)
	fracPart := f - float64(intPart)
	if fracPart < 0 {
		fracPart = -fracPart
	}

	result := formatInt(intPart)

	if precision > 0 {
		result += "."
		for i := 0; i < precision; i++ {
			fracPart *= 10
			digit := int(fracPart)
			result += string('0' + byte(digit))
			fracPart -= float64(digit)
		}
	}

	return result
}
