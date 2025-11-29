import { useState, useEffect } from 'react';
import {
  Server,
  Database,
  Cpu,
  HardDrive,
  Activity,
  RefreshCw,
  Clock,
  Layers,
  TrendingUp,
  AlertCircle,
  CheckCircle,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card, CardTitle, CardContent } from '../components/Card';
import { MetricCard } from '../components/MetricCard';
import { StatusBadge } from '../components/StatusBadge';
import { Button } from '../components/Button';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '../components/Table';
import { formatBytes, formatUptime } from '../lib/utils';
import { endpoints } from '../lib/api';

export function Infrastructure() {
  const [health, setHealth] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [lastRefresh, setLastRefresh] = useState(new Date());

  const fetchHealth = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await endpoints.getInfrastructureHealth();
      setHealth(data);
      setLastRefresh(new Date());
    } catch (err) {
      console.error('Failed to fetch infrastructure health:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchHealth();
    // Refresh every 15 seconds
    const interval = setInterval(fetchHealth, 15000);
    return () => clearInterval(interval);
  }, []);

  const getStatusColor = (status) => {
    switch (status) {
      case 'healthy':
        return 'healthy';
      case 'degraded':
        return 'degraded';
      case 'error':
      case 'down':
        return 'down';
      default:
        return 'offline';
    }
  };

  const getPoolUsagePercent = (pool) => {
    if (!pool || pool.max_connections === 0) return 0;
    return ((pool.acquired_connections / pool.max_connections) * 100).toFixed(0);
  };

  const getPoolStatus = (pool) => {
    if (!pool) return 'offline';
    const usage = pool.acquired_connections / pool.max_connections;
    if (usage > 0.9) return 'down';
    if (usage > 0.7) return 'degraded';
    return 'healthy';
  };

  return (
    <>
      <PageHeader
        title="Infrastructure"
        description="Control plane and database health metrics"
        actions={
          <div className="flex items-center gap-3">
            <span className="text-xs text-theme-muted">
              Updated {lastRefresh.toLocaleTimeString()}
            </span>
            <Button
              variant="secondary"
              size="sm"
              onClick={fetchHealth}
              disabled={loading}
            >
              <RefreshCw className={`w-4 h-4 mr-1.5 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </Button>
          </div>
        }
      />

      <PageContent>
        {error && (
          <Card className="border-red-500/50 bg-red-500/10 mb-6">
            <CardContent className="flex items-center gap-3 py-3">
              <AlertCircle className="w-5 h-5 text-red-400" />
              <span className="text-red-400">{error}</span>
            </CardContent>
          </Card>
        )}

        {/* Control Plane Section */}
        <div className="mb-6">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-2">
              <Server className="w-5 h-5 text-theme-muted" />
              <h2 className="text-lg font-medium text-theme-primary">Control Plane</h2>
            </div>
            {health?.control_plane && (
              <StatusBadge status={getStatusColor(health.control_plane.status)} />
            )}
          </div>

          <div className="grid grid-cols-2 md:grid-cols-5 gap-4">
            <MetricCard
              title="CPU Usage"
              value={health?.control_plane?.cpu_percent?.toFixed(1) ?? '-'}
              unit="%"
              icon={Cpu}
              status={health?.control_plane?.cpu_percent > 80 ? 'degraded' : 'healthy'}
            />
            <MetricCard
              title="Memory"
              value={health?.control_plane?.memory_mb?.toFixed(0) ?? '-'}
              unit="MB"
              subtitle={health?.control_plane?.memory_percent ? `${health.control_plane.memory_percent.toFixed(1)}% used` : ''}
              icon={HardDrive}
              status={health?.control_plane?.memory_percent > 80 ? 'degraded' : 'healthy'}
            />
            <MetricCard
              title="Goroutines"
              value={health?.control_plane?.goroutines ?? '-'}
              icon={Activity}
            />
            <MetricCard
              title="Uptime"
              value={health?.control_plane?.uptime_seconds ? formatUptime(health.control_plane.uptime_seconds) : '-'}
              icon={Clock}
            />
            <MetricCard
              title="Status"
              value={health?.control_plane?.status ?? 'unknown'}
              icon={health?.control_plane?.status === 'healthy' ? CheckCircle : AlertCircle}
            />
          </div>
        </div>

        {/* Database Section */}
        <div className="mb-6">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-2">
              <Database className="w-5 h-5 text-theme-muted" />
              <h2 className="text-lg font-medium text-theme-primary">Database (TimescaleDB)</h2>
            </div>
            {health?.database && (
              <StatusBadge status={getStatusColor(health.database.status)} />
            )}
          </div>

          <div className="grid grid-cols-2 md:grid-cols-6 gap-4 mb-4">
            <MetricCard
              title="Connection Pool"
              value={`${health?.database?.pool?.acquired_connections ?? 0}/${health?.database?.pool?.max_connections ?? 0}`}
              subtitle={`${getPoolUsagePercent(health?.database?.pool)}% in use`}
              icon={Layers}
              status={getPoolStatus(health?.database?.pool)}
            />
            <MetricCard
              title="Database Size"
              value={health?.database?.size_formatted ?? '-'}
              icon={Database}
            />
            <MetricCard
              title="Compression"
              value={health?.database?.compression_ratio?.toFixed(1) ?? '-'}
              unit="x"
              subtitle="storage reduction"
              icon={TrendingUp}
            />
            <MetricCard
              title="Daily Growth"
              value={health?.storage_forecast?.daily_growth_formatted ?? '-'}
              icon={TrendingUp}
            />
            <MetricCard
              title="30-Day Projected"
              value={health?.storage_forecast?.projected_30d_formatted ?? '-'}
              icon={TrendingUp}
            />
            <MetricCard
              title="Days to 100GB"
              value={health?.storage_forecast?.days_until_100gb === -1 ? 'N/A' : (health?.storage_forecast?.days_until_100gb ?? '-')}
              icon={Clock}
              status={health?.storage_forecast?.days_until_100gb !== undefined && health?.storage_forecast?.days_until_100gb < 30 ? 'degraded' : undefined}
            />
          </div>

          {/* Table Sizes */}
          {health?.database?.tables && health.database.tables.length > 0 && (
            <Card>
              <CardTitle>Table Sizes</CardTitle>
              <CardContent>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Table</TableHead>
                      <TableHead className="text-right">Size</TableHead>
                      <TableHead className="text-right">Compression</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {health.database.tables.map((table) => (
                      <TableRow key={table.name}>
                        <TableCell className="font-mono text-sm">{table.name}</TableCell>
                        <TableCell className="text-right tabular-nums">{table.size_formatted}</TableCell>
                        <TableCell className="text-right tabular-nums">
                          {table.compression_ratio > 0 ? `${table.compression_ratio.toFixed(1)}x` : '-'}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          )}
        </div>

        {/* Buffer Section (Redis) */}
        {health?.buffer?.enabled && (
          <div className="mb-6">
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-2">
                <Activity className="w-5 h-5 text-theme-muted" />
                <h2 className="text-lg font-medium text-theme-primary">Buffer (Redis)</h2>
              </div>
              <StatusBadge status={health.buffer.connected ? 'healthy' : 'down'} />
            </div>

            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <MetricCard
                title="Queue Depth"
                value={health.buffer.queue_depth?.toLocaleString() ?? '-'}
                icon={Layers}
                status={health.buffer.queue_depth > 100000 ? 'degraded' : 'healthy'}
              />
              <MetricCard
                title="Flush Rate"
                value={health.buffer.flush_rate?.toFixed(0) ?? '-'}
                unit="/sec"
                icon={TrendingUp}
              />
              <MetricCard
                title="Connected"
                value={health.buffer.connected ? 'Yes' : 'No'}
                icon={health.buffer.connected ? CheckCircle : AlertCircle}
              />
              <MetricCard
                title="Status"
                value={health.buffer.connected ? 'Healthy' : 'Disconnected'}
                icon={health.buffer.connected ? CheckCircle : AlertCircle}
              />
            </div>
          </div>
        )}

        {/* Retention Policy Section */}
        {health?.storage_forecast?.retention_policy && (
          <div className="mb-6">
            <div className="flex items-center gap-2 mb-4">
              <Clock className="w-5 h-5 text-theme-muted" />
              <h2 className="text-lg font-medium text-theme-primary">Retention Policies</h2>
            </div>

            <Card>
              <CardContent>
                <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
                  <div>
                    <p className="text-xs text-theme-muted uppercase tracking-wide mb-1">Raw Data</p>
                    <p className="text-xl font-semibold text-theme-primary">
                      {health.storage_forecast.retention_policy.raw_data_days} days
                    </p>
                  </div>
                  <div>
                    <p className="text-xs text-theme-muted uppercase tracking-wide mb-1">Hourly Aggregates</p>
                    <p className="text-xl font-semibold text-theme-primary">
                      {health.storage_forecast.retention_policy.hourly_aggregate_days} days
                    </p>
                  </div>
                  <div>
                    <p className="text-xs text-theme-muted uppercase tracking-wide mb-1">Daily Aggregates</p>
                    <p className="text-xl font-semibold text-theme-primary">
                      {health.storage_forecast.retention_policy.daily_aggregate_days} days
                    </p>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        )}
      </PageContent>
    </>
  );
}
