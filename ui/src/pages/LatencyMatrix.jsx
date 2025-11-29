import { useState, useEffect, useMemo } from 'react';
import {
  Grid,
  RefreshCw,
  AlertTriangle,
  MapPin,
  Clock,
  ArrowRight,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card, CardTitle, CardContent } from '../components/Card';
import { Button } from '../components/Button';
import { endpoints } from '../lib/api';

const timeWindows = [
  { value: '1h', label: '1 hour' },
  { value: '6h', label: '6 hours' },
  { value: '24h', label: '24 hours' },
  { value: '168h', label: '7 days' },
];

// Color scale for latency values (green -> yellow -> red)
function getLatencyColor(latency, isInMarket) {
  if (latency === null || latency === undefined) return 'bg-surface-tertiary';

  if (isInMarket) {
    // For in-market, 5ms is the SLA threshold
    if (latency <= 2) return 'bg-emerald-500/80';
    if (latency <= 5) return 'bg-emerald-400/60';
    if (latency <= 10) return 'bg-yellow-500/60';
    if (latency <= 20) return 'bg-orange-500/60';
    return 'bg-red-500/60';
  } else {
    // For cross-market, scale is different
    if (latency <= 20) return 'bg-emerald-500/60';
    if (latency <= 50) return 'bg-emerald-400/40';
    if (latency <= 100) return 'bg-yellow-500/50';
    if (latency <= 150) return 'bg-orange-500/50';
    return 'bg-red-500/50';
  }
}

function getLatencyTextColor(latency, isInMarket) {
  if (latency === null || latency === undefined) return 'text-theme-muted';

  if (isInMarket) {
    if (latency <= 5) return 'text-emerald-100';
    if (latency <= 10) return 'text-yellow-100';
    return 'text-red-100';
  } else {
    if (latency <= 50) return 'text-emerald-100';
    if (latency <= 100) return 'text-yellow-100';
    return 'text-red-100';
  }
}

export function LatencyMatrix() {
  const [matrixData, setMatrixData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [timeWindow, setTimeWindow] = useState('24h');
  const [lastRefresh, setLastRefresh] = useState(null);
  const [hoveredCell, setHoveredCell] = useState(null);

  const fetchMatrix = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await endpoints.getLatencyMatrix(timeWindow);
      setMatrixData(res);
      setLastRefresh(new Date());
    } catch (err) {
      console.error('Failed to fetch latency matrix:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchMatrix();
  }, [timeWindow]);

  // Build matrix from data
  const { matrix, sourceRegions, targetRegions } = useMemo(() => {
    if (!matrixData?.cells || matrixData.cells.length === 0) {
      return { matrix: {}, sourceRegions: [], targetRegions: [] };
    }

    const srcSet = new Set();
    const tgtSet = new Set();
    const mtx = {};

    matrixData.cells.forEach(cell => {
      srcSet.add(cell.agent_region);
      tgtSet.add(cell.target_region);

      const key = `${cell.agent_region}|${cell.target_region}`;
      mtx[key] = {
        avgLatency: cell.avg_latency_ms,
        p95Latency: cell.p95_latency_ms,
        probeCount: cell.probe_count,
        isInMarket: cell.agent_region?.toLowerCase() === cell.target_region?.toLowerCase(),
      };
    });

    // Sort regions alphabetically
    const srcRegions = Array.from(srcSet).sort();
    const tgtRegions = Array.from(tgtSet).sort();

    return { matrix: mtx, sourceRegions: srcRegions, targetRegions: tgtRegions };
  }, [matrixData]);

  // Calculate statistics
  const stats = useMemo(() => {
    if (!matrixData?.cells || matrixData.cells.length === 0) {
      return { avgInMarket: null, avgCrossMarket: null, totalProbes: 0 };
    }

    let inMarketTotal = 0;
    let inMarketCount = 0;
    let crossMarketTotal = 0;
    let crossMarketCount = 0;
    let totalProbes = 0;

    matrixData.cells.forEach(cell => {
      totalProbes += cell.probe_count || 0;
      const isInMarket = cell.agent_region?.toLowerCase() === cell.target_region?.toLowerCase();
      if (isInMarket) {
        inMarketTotal += cell.avg_latency_ms * (cell.probe_count || 1);
        inMarketCount += cell.probe_count || 1;
      } else {
        crossMarketTotal += cell.avg_latency_ms * (cell.probe_count || 1);
        crossMarketCount += cell.probe_count || 1;
      }
    });

    return {
      avgInMarket: inMarketCount > 0 ? inMarketTotal / inMarketCount : null,
      avgCrossMarket: crossMarketCount > 0 ? crossMarketTotal / crossMarketCount : null,
      totalProbes,
    };
  }, [matrixData]);

  if (error) {
    return (
      <>
        <PageHeader title="Latency Matrix" />
        <PageContent>
          <Card accent="red">
            <div className="flex items-center gap-3">
              <AlertTriangle className="w-6 h-6 text-pilot-red" />
              <div>
                <h3 className="font-medium text-theme-primary">Failed to load data</h3>
                <p className="text-sm text-theme-muted">{error}</p>
              </div>
              <Button variant="secondary" size="sm" onClick={fetchMatrix} className="ml-auto">
                Retry
              </Button>
            </div>
          </Card>
        </PageContent>
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="Latency Matrix"
        description="Region-to-region latency heatmap"
        actions={
          <div className="flex items-center gap-3">
            <select
              value={timeWindow}
              onChange={(e) => setTimeWindow(e.target.value)}
              className="px-3 py-1.5 text-sm bg-surface-primary border border-theme rounded-lg text-theme-primary focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
            >
              {timeWindows.map(tw => (
                <option key={tw.value} value={tw.value}>{tw.label}</option>
              ))}
            </select>
            <Button variant="secondary" size="sm" onClick={fetchMatrix} className="gap-2">
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </Button>
          </div>
        }
      />

      <PageContent>
        {/* Summary Stats */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
          <Card>
            <div className="flex items-center gap-3">
              <div className="p-2 bg-status-healthy/20 rounded-lg">
                <MapPin className="w-5 h-5 text-status-healthy" />
              </div>
              <div>
                <p className="text-sm text-theme-muted">In-Market Avg</p>
                <p className="text-xl font-semibold text-theme-primary">
                  {stats.avgInMarket != null ? `${stats.avgInMarket.toFixed(2)}ms` : 'N/A'}
                </p>
                {stats.avgInMarket != null && (
                  <p className={`text-xs ${stats.avgInMarket <= 5 ? 'text-status-healthy' : 'text-pilot-red'}`}>
                    {stats.avgInMarket <= 5 ? 'Within SLA' : 'SLA Breach'}
                  </p>
                )}
              </div>
            </div>
          </Card>

          <Card>
            <div className="flex items-center gap-3">
              <div className="p-2 bg-pilot-cyan/20 rounded-lg">
                <ArrowRight className="w-5 h-5 text-pilot-cyan" />
              </div>
              <div>
                <p className="text-sm text-theme-muted">Cross-Market Avg</p>
                <p className="text-xl font-semibold text-theme-primary">
                  {stats.avgCrossMarket != null ? `${stats.avgCrossMarket.toFixed(1)}ms` : 'N/A'}
                </p>
              </div>
            </div>
          </Card>

          <Card>
            <div className="flex items-center gap-3">
              <div className="p-2 bg-accent/20 rounded-lg">
                <Grid className="w-5 h-5 text-accent" />
              </div>
              <div>
                <p className="text-sm text-theme-muted">Regions</p>
                <p className="text-xl font-semibold text-theme-primary">
                  {sourceRegions.length} x {targetRegions.length}
                </p>
              </div>
            </div>
          </Card>

          <Card>
            <div className="flex items-center gap-3">
              <div className="p-2 bg-surface-tertiary rounded-lg">
                <Clock className="w-5 h-5 text-theme-muted" />
              </div>
              <div>
                <p className="text-sm text-theme-muted">Last Updated</p>
                <p className="text-xl font-semibold text-theme-primary">
                  {lastRefresh ? lastRefresh.toLocaleTimeString() : 'Never'}
                </p>
              </div>
            </div>
          </Card>
        </div>

        {/* Matrix */}
        <Card>
          <div className="flex items-center justify-between mb-4">
            <CardTitle>Region Latency Matrix</CardTitle>
            <div className="flex items-center gap-4 text-xs">
              <span className="flex items-center gap-1.5">
                <span className="w-3 h-3 rounded bg-emerald-500/80"></span>
                <span className="text-theme-muted">&lt;5ms</span>
              </span>
              <span className="flex items-center gap-1.5">
                <span className="w-3 h-3 rounded bg-yellow-500/60"></span>
                <span className="text-theme-muted">5-20ms</span>
              </span>
              <span className="flex items-center gap-1.5">
                <span className="w-3 h-3 rounded bg-orange-500/60"></span>
                <span className="text-theme-muted">20-50ms</span>
              </span>
              <span className="flex items-center gap-1.5">
                <span className="w-3 h-3 rounded bg-red-500/60"></span>
                <span className="text-theme-muted">&gt;50ms</span>
              </span>
              <span className="flex items-center gap-1.5">
                <span className="w-3 h-3 rounded bg-surface-tertiary border border-theme"></span>
                <span className="text-theme-muted">No data</span>
              </span>
            </div>
          </div>
          <CardContent>
            {loading ? (
              <div className="flex items-center justify-center py-12">
                <RefreshCw className="w-6 h-6 animate-spin text-theme-muted" />
              </div>
            ) : sourceRegions.length === 0 || targetRegions.length === 0 ? (
              <div className="text-center py-12 text-theme-muted">
                <Grid className="w-12 h-12 mx-auto mb-3 opacity-50" />
                <p>No latency data available</p>
                <p className="text-sm mt-1">Data will appear once agents report probe results with region information</p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full border-collapse">
                  <thead>
                    <tr>
                      <th className="p-2 text-left text-xs font-medium text-theme-muted sticky left-0 bg-surface-secondary z-10">
                        <div className="flex items-center gap-1">
                          <span>Agent</span>
                          <ArrowRight className="w-3 h-3" />
                          <span>Target</span>
                        </div>
                      </th>
                      {targetRegions.map(region => (
                        <th
                          key={region}
                          className="p-2 text-center text-xs font-medium text-theme-muted whitespace-nowrap"
                        >
                          {region || 'Unknown'}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {sourceRegions.map(srcRegion => (
                      <tr key={srcRegion}>
                        <td className="p-2 text-xs font-medium text-theme-primary whitespace-nowrap sticky left-0 bg-surface-secondary z-10">
                          {srcRegion || 'Unknown'}
                        </td>
                        {targetRegions.map(tgtRegion => {
                          const key = `${srcRegion}|${tgtRegion}`;
                          const cell = matrix[key];
                          const isInMarket = srcRegion?.toLowerCase() === tgtRegion?.toLowerCase();
                          const isHovered = hoveredCell === key;

                          return (
                            <td
                              key={tgtRegion}
                              className="p-1"
                              onMouseEnter={() => setHoveredCell(key)}
                              onMouseLeave={() => setHoveredCell(null)}
                            >
                              <div
                                className={`
                                  relative min-w-[60px] h-12 rounded flex flex-col items-center justify-center cursor-default
                                  transition-all duration-150
                                  ${getLatencyColor(cell?.avgLatency, isInMarket)}
                                  ${isInMarket ? 'ring-2 ring-status-healthy/50' : ''}
                                  ${isHovered ? 'ring-2 ring-pilot-cyan scale-105 z-20' : ''}
                                `}
                              >
                                <span className={`text-sm font-mono font-medium ${getLatencyTextColor(cell?.avgLatency, isInMarket)}`}>
                                  {cell?.avgLatency != null ? `${cell.avgLatency.toFixed(1)}` : '—'}
                                </span>
                                {cell?.avgLatency != null && (
                                  <span className={`text-[10px] ${getLatencyTextColor(cell?.avgLatency, isInMarket)} opacity-75`}>
                                    ms
                                  </span>
                                )}
                              </div>

                              {/* Tooltip */}
                              {isHovered && cell && (
                                <div className="absolute z-50 bg-surface-secondary border border-theme rounded-lg shadow-xl p-3 text-xs min-w-[160px] -translate-x-1/2 left-1/2 mt-1">
                                  <div className="font-medium text-theme-primary mb-2">
                                    {srcRegion} → {tgtRegion}
                                  </div>
                                  <div className="space-y-1">
                                    <div className="flex justify-between">
                                      <span className="text-theme-muted">Avg Latency:</span>
                                      <span className="text-theme-primary font-mono">{cell.avgLatency?.toFixed(2)}ms</span>
                                    </div>
                                    <div className="flex justify-between">
                                      <span className="text-theme-muted">P95 Latency:</span>
                                      <span className="text-theme-primary font-mono">{cell.p95Latency?.toFixed(2)}ms</span>
                                    </div>
                                    <div className="flex justify-between">
                                      <span className="text-theme-muted">Probes:</span>
                                      <span className="text-theme-primary">{cell.probeCount?.toLocaleString()}</span>
                                    </div>
                                    {isInMarket && (
                                      <div className="mt-2 pt-2 border-t border-theme">
                                        <span className={`text-xs ${cell.avgLatency <= 5 ? 'text-status-healthy' : 'text-pilot-red'}`}>
                                          {cell.avgLatency <= 5 ? 'SLA: OK' : 'SLA: BREACH'}
                                        </span>
                                      </div>
                                    )}
                                  </div>
                                </div>
                              )}
                            </td>
                          );
                        })}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </CardContent>
        </Card>

        {/* Legend */}
        <div className="mt-4 text-xs text-theme-muted">
          <p>
            <strong>In-market</strong> cells (diagonal, highlighted with green ring) show latency between agents and targets in the same region.
            SLA threshold is 5ms for in-market latency.
          </p>
          <p className="mt-1">
            <strong>Cross-market</strong> cells show latency between agents and targets in different regions.
          </p>
        </div>
      </PageContent>
    </>
  );
}
