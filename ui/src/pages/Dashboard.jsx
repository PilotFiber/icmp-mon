import { useState, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Activity,
  Server,
  Target,
  AlertTriangle,
  Clock,
  RefreshCw,
  AlertCircle,
  MapPin,
  Cloud,
  Zap,
  TrendingUp,
  TrendingDown,
  CheckCircle,
  XCircle,
} from 'lucide-react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  AreaChart,
  Area,
  BarChart,
  Bar,
  Cell,
  ReferenceLine,
} from 'recharts';

import { PageHeader, PageContent } from '../components/Layout';
import { Card, CardTitle, CardContent } from '../components/Card';
import { MetricCard } from '../components/MetricCard';
import { StatusBadge, StatusDot } from '../components/StatusBadge';
import { AlertCard } from '../components/Alert';
import { Button } from '../components/Button';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '../components/Table';
import { endpoints } from '../lib/api';
import { formatRelativeTime } from '../lib/utils';

export function Dashboard() {
  const navigate = useNavigate();
  const [fleetOverview, setFleetOverview] = useState(null);
  const [agents, setAgents] = useState([]);
  const [targets, setTargets] = useState([]);
  const [targetStatuses, setTargetStatuses] = useState({});
  const [tiers, setTiers] = useState([]);
  const [incidents, setIncidents] = useState([]);
  const [latencyHistory, setLatencyHistory] = useState([]);
  const [inMarketLatency, setInMarketLatency] = useState({ avg: null, history: [] });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [lastRefresh, setLastRefresh] = useState(new Date());

  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);

      // Use fleet overview for stats + fewer additional calls
      const [overviewRes, agentsRes, tiersRes, latencyRes, inMarketRes, incidentsRes] = await Promise.all([
        endpoints.getFleetOverview().catch(() => null),
        endpoints.listAgents(),
        endpoints.listTiers(),
        endpoints.getLatencyTrend('1h').catch(() => ({ history: [] })),
        endpoints.getInMarketLatencyTrend('1h').catch(() => ({ history: [] })),
        endpoints.listIncidents('active', 10).catch(() => ({ incidents: [] })),
      ]);

      setFleetOverview(overviewRes);
      setAgents(agentsRes.agents || []);
      setTiers(tiersRes.tiers || []);
      setIncidents(incidentsRes.incidents || []);

      // Only fetch targets/statuses if needed for detailed display
      // For now, we still need them for the targets table
      const [targetsRes, statusesRes] = await Promise.all([
        endpoints.listTargets(),
        endpoints.getAllTargetStatuses().catch(() => ({ statuses: [] })),
      ]);

      setTargets(targetsRes.targets || []);

      // Convert statuses array to map by target_id
      const statusMap = {};
      (statusesRes.statuses || []).forEach(s => {
        statusMap[s.target_id] = s;
      });
      setTargetStatuses(statusMap);

      // Format latency history for chart
      const history = (latencyRes.history || []).map(point => ({
        time: new Date(point.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
        latency: point.avg_latency_ms?.toFixed(1) || null,
        p95: point.max_latency_ms?.toFixed(1) || null,
      }));
      setLatencyHistory(history);

      // Process in-market latency data
      const inMarketHistory = (inMarketRes.history || []).map(point => ({
        time: new Date(point.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
        latency: point.avg_latency_ms?.toFixed(2) || null,
        p95: point.max_latency_ms?.toFixed(2) || null,
      }));
      // Calculate current in-market avg from last few data points
      const recentPoints = inMarketHistory.slice(-5).filter(p => p.latency);
      const avgInMarket = recentPoints.length > 0
        ? (recentPoints.reduce((sum, p) => sum + parseFloat(p.latency), 0) / recentPoints.length).toFixed(2)
        : null;
      setInMarketLatency({ avg: avgInMarket, history: inMarketHistory });

      setLastRefresh(new Date());
    } catch (err) {
      console.error('Failed to fetch data:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();

    // Refresh every 30 seconds (was 10 seconds)
    const interval = setInterval(fetchData, 30000);
    return () => clearInterval(interval);
  }, []);

  // Filter out archived agents for display
  const nonArchivedAgents = useMemo(() => {
    return agents.filter(agent => !agent.archived_at);
  }, [agents]);

  // Calculate stats - prefer fleet overview when available, but use nonArchivedAgents for fallback
  const totalTargets = fleetOverview?.total_targets ?? targets.length;
  const totalAgents = fleetOverview?.total_agents ?? nonArchivedAgents.length;
  const activeAgents = fleetOverview?.active_agents ?? nonArchivedAgents.filter(a => a.status === 'active').length;
  const activeIncidents = incidents.filter(i => i.status === 'active' || i.status === 'acknowledged').length;

  // Health stats from fleet overview
  const healthStats = useMemo(() => {
    if (fleetOverview) {
      return {
        monitorable: fleetOverview.monitorable_targets || 0,
        healthy: fleetOverview.healthy_targets || 0,
        percentage: fleetOverview.health_percentage || 0,
      };
    }
    // Fallback to calculated stats
    const healthy = Object.values(targetStatuses).filter(s => s.status === 'healthy').length;
    const monitorable = Object.values(targetStatuses).length;
    return {
      monitorable,
      healthy,
      percentage: monitorable > 0 ? (healthy / monitorable * 100) : 0,
    };
  }, [fleetOverview, targetStatuses]);

  // Calculate target health stats for backwards compat
  const targetStats = useMemo(() => {
    const healthy = Object.values(targetStatuses).filter(s => s.status === 'healthy').length;
    const degraded = Object.values(targetStatuses).filter(s => s.status === 'degraded').length;
    const down = Object.values(targetStatuses).filter(s => s.status === 'down').length;
    const unknown = totalTargets - healthy - degraded - down;
    return { healthy, degraded, down, unknown };
  }, [targetStatuses, totalTargets]);

  // Group agents by provider/region (excluding archived)
  const agentsByProvider = useMemo(() => {
    const grouped = {};
    nonArchivedAgents.forEach(agent => {
      const provider = agent.provider || 'unknown';
      if (!grouped[provider]) {
        grouped[provider] = { total: 0, active: 0, agents: [] };
      }
      grouped[provider].total++;
      if (agent.status === 'active') grouped[provider].active++;
      grouped[provider].agents.push(agent);
    });
    return grouped;
  }, [nonArchivedAgents]);

  // Group agents by region (excluding archived)
  const agentsByRegion = useMemo(() => {
    const grouped = {};
    nonArchivedAgents.forEach(agent => {
      const region = agent.region || 'unknown';
      if (!grouped[region]) {
        grouped[region] = { total: 0, active: 0 };
      }
      grouped[region].total++;
      if (agent.status === 'active') grouped[region].active++;
    });
    return Object.entries(grouped).map(([region, data]) => ({
      region,
      ...data,
    }));
  }, [nonArchivedAgents]);

  // Group targets by tier
  const tierBreakdown = tiers.map(tier => {
    const tierTargets = targets.filter(t => t.tier === tier.name);
    const tierStatuses = tierTargets.map(t => targetStatuses[t.id]?.status);
    const healthy = tierStatuses.filter(s => s === 'healthy').length;
    const down = tierStatuses.filter(s => s === 'down').length;
    return {
      tier: tier.name,
      displayName: tier.display_name,
      count: tierTargets.length,
      healthy,
      down,
      interval: tier.probe_interval ? `${tier.probe_interval / 1000000000}s` : 'N/A',
    };
  });

  if (error) {
    return (
      <>
        <PageHeader title="Fleet Overview" />
        <PageContent>
          <Card accent="red">
            <div className="flex items-center gap-3">
              <AlertTriangle className="w-6 h-6 text-pilot-red" />
              <div>
                <h3 className="font-medium text-theme-primary">Failed to load data</h3>
                <p className="text-sm text-theme-muted">{error}</p>
              </div>
              <Button variant="secondary" size="sm" onClick={fetchData} className="ml-auto">
                Retry
              </Button>
            </div>
          </Card>
        </PageContent>
      </>
    );
  }

  // Provider icons
  const providerIcons = {
    pilot: Zap,
    aws: Cloud,
    digitalocean: Cloud,
    default: Server,
  };

  const getProviderIcon = (provider) => providerIcons[provider?.toLowerCase()] || providerIcons.default;

  return (
    <>
      <PageHeader
        title="Fleet Overview"
        description="Real-time network monitoring across all agents"
        actions={
          <Button variant="secondary" size="sm" onClick={fetchData} className="gap-2">
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </Button>
        }
      />

      <PageContent>
        {/* Active Incidents Alert */}
        {activeIncidents > 0 && (
          <div
            className="mb-6 bg-pilot-red/10 border border-pilot-red/30 rounded-lg p-4 cursor-pointer hover:bg-pilot-red/20 transition-colors"
            onClick={() => navigate('/incidents')}
          >
            <div className="flex items-center gap-3">
              <AlertCircle className="w-6 h-6 text-pilot-red" />
              <div className="flex-1">
                <h3 className="font-medium text-theme-primary">
                  {activeIncidents} Active Incident{activeIncidents !== 1 ? 's' : ''}
                </h3>
                <p className="text-sm text-theme-muted">Click to view and manage incidents</p>
              </div>
              <Button variant="danger" size="sm">View Incidents</Button>
            </div>
          </div>
        )}

        {/* Top Metrics */}
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3 md:gap-4 mb-4 md:mb-6">
          <MetricCard
            title="Active Agents"
            value={`${activeAgents}/${totalAgents}`}
            icon={Server}
            status={activeAgents < totalAgents ? 'degraded' : 'healthy'}
          />
          <MetricCard
            title="Total Targets"
            value={totalTargets.toLocaleString()}
            icon={Target}
          />
          <MetricCard
            title="Health"
            value={`${healthStats.percentage.toFixed(1)}%`}
            subtitle={`${healthStats.healthy.toLocaleString()} / ${healthStats.monitorable.toLocaleString()}`}
            status={healthStats.percentage >= 99 ? 'healthy' : healthStats.percentage >= 95 ? 'degraded' : 'down'}
          />
          <MetricCard
            title="In-Market Latency"
            value={inMarketLatency.avg ? `${inMarketLatency.avg}ms` : 'N/A'}
            subtitle={inMarketLatency.avg ? (
              <span className="flex items-center gap-1">
                {parseFloat(inMarketLatency.avg) <= 5 ? (
                  <>
                    <CheckCircle className="w-3 h-3 text-status-healthy" />
                    <span className="text-status-healthy">SLA Met</span>
                  </>
                ) : (
                  <>
                    <XCircle className="w-3 h-3 text-pilot-red" />
                    <span className="text-pilot-red">SLA Breach</span>
                  </>
                )}
              </span>
            ) : 'No data'}
            icon={Activity}
            status={inMarketLatency.avg ? (parseFloat(inMarketLatency.avg) <= 5 ? 'healthy' : 'down') : null}
          />
          <MetricCard
            title="Degraded/Down"
            value={`${targetStats.degraded}/${targetStats.down}`}
            status={targetStats.down > 0 ? 'down' : targetStats.degraded > 0 ? 'degraded' : 'healthy'}
          />
          <MetricCard
            title="Last Updated"
            value={lastRefresh.toLocaleTimeString()}
            icon={Clock}
          />
        </div>

        {/* Status Overview */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 md:gap-6 mb-4 md:mb-6">
          {/* Agents by Provider Card */}
          <Card className="lg:col-span-1">
            <div className="flex items-center justify-between mb-4">
              <CardTitle>Agents by Provider</CardTitle>
              <a
                href="/agents"
                className="text-sm text-pilot-cyan hover:text-pilot-cyan-light transition-colors"
              >
                View all
              </a>
            </div>
            <CardContent>
              {nonArchivedAgents.length === 0 ? (
                <div className="text-center py-8 text-theme-muted">
                  <Server className="w-12 h-12 mx-auto mb-3 opacity-50" />
                  <p>No agents registered</p>
                  <p className="text-sm mt-1">Start an agent to begin monitoring</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {Object.entries(agentsByProvider).map(([provider, data]) => {
                    const ProviderIcon = getProviderIcon(provider);
                    return (
                      <div key={provider} className="bg-surface-primary rounded-lg p-3">
                        <div className="flex items-center justify-between mb-2">
                          <div className="flex items-center gap-2">
                            <ProviderIcon className="w-4 h-4 text-pilot-cyan" />
                            <span className="font-medium text-theme-primary capitalize">{provider}</span>
                          </div>
                          <span className={`text-sm ${data.active < data.total ? 'text-warning' : 'text-status-healthy'}`}>
                            {data.active}/{data.total} active
                          </span>
                        </div>
                        <div className="space-y-1">
                          {data.agents.map(agent => (
                            <div key={agent.id} className="flex items-center justify-between text-sm">
                              <div className="flex items-center gap-2">
                                <StatusDot
                                  status={agent.status === 'active' ? 'healthy' : 'down'}
                                  pulse={agent.status === 'active'}
                                  size="sm"
                                />
                                <span className="text-theme-secondary">{agent.name}</span>
                              </div>
                              <span className="text-xs text-theme-muted">{agent.location || agent.region}</span>
                            </div>
                          ))}
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </CardContent>
          </Card>

          {/* Latency Chart */}
          <Card className="lg:col-span-2">
            <div className="flex items-center justify-between mb-2">
              <CardTitle>In-Market Latency (1h)</CardTitle>
              <div className="flex items-center gap-4 text-xs">
                <span className="flex items-center gap-1.5">
                  <span className="w-3 h-0.5 bg-[#10B981]"></span>
                  <span className="text-theme-muted">In-Market Avg</span>
                </span>
                <span className="flex items-center gap-1.5">
                  <span className="w-3 h-0.5 bg-[#6EDBE0] opacity-50"></span>
                  <span className="text-theme-muted">All Traffic</span>
                </span>
                <span className="flex items-center gap-1.5">
                  <span className="w-3 h-0.5 bg-[#FC534E]" style={{ borderTop: '1px dashed #FC534E' }}></span>
                  <span className="text-theme-muted">5ms SLA</span>
                </span>
              </div>
            </div>
            <CardContent className="mt-4 h-48 sm:h-56 md:h-64">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={inMarketLatency.history.length > 0 ? inMarketLatency.history : latencyHistory}>
                  <defs>
                    <linearGradient id="inMarketGradient" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#10B981" stopOpacity={0.3}/>
                      <stop offset="95%" stopColor="#10B981" stopOpacity={0}/>
                    </linearGradient>
                    <linearGradient id="latencyGradient" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#6EDBE0" stopOpacity={0.15}/>
                      <stop offset="95%" stopColor="#6EDBE0" stopOpacity={0}/>
                    </linearGradient>
                  </defs>
                  <XAxis
                    dataKey="time"
                    stroke="#6B7280"
                    fontSize={12}
                    tickLine={false}
                    axisLine={false}
                  />
                  <YAxis
                    stroke="#6B7280"
                    fontSize={12}
                    tickLine={false}
                    axisLine={false}
                    tickFormatter={(v) => `${v}ms`}
                    domain={[0, 'auto']}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: '#18284F',
                      border: '1px solid #2A3D6B',
                      borderRadius: '8px',
                    }}
                    labelStyle={{ color: '#9CA3AF' }}
                    formatter={(value, name) => [`${value}ms`, name]}
                  />
                  <ReferenceLine
                    y={5}
                    stroke="#FC534E"
                    strokeDasharray="5 5"
                    strokeWidth={1}
                    label={{
                      value: '5ms SLA',
                      position: 'right',
                      fill: '#FC534E',
                      fontSize: 10,
                    }}
                  />
                  <Area
                    type="monotone"
                    dataKey="latency"
                    stroke="#10B981"
                    strokeWidth={2}
                    fill="url(#inMarketGradient)"
                    name="In-Market Avg"
                  />
                  <Line
                    type="monotone"
                    dataKey="p95"
                    stroke="#6EDBE0"
                    strokeWidth={1}
                    strokeOpacity={0.5}
                    dot={false}
                    name="P95"
                  />
                </AreaChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>
        </div>

        {/* Region Coverage & Tier Breakdown */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 md:gap-6 mb-4 md:mb-6">
          {/* Region Coverage */}
          <Card>
            <CardTitle>Agent Coverage by Region</CardTitle>
            <CardContent className="mt-4">
              {agentsByRegion.length === 0 ? (
                <div className="text-center py-8 text-theme-muted">
                  <MapPin className="w-12 h-12 mx-auto mb-3 opacity-50" />
                  <p>No agents registered</p>
                </div>
              ) : (
                <div className="h-48">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={agentsByRegion} layout="vertical">
                      <XAxis type="number" stroke="#6B7280" fontSize={12} tickLine={false} axisLine={false} />
                      <YAxis type="category" dataKey="region" stroke="#6B7280" fontSize={12} tickLine={false} axisLine={false} width={80} />
                      <Tooltip
                        contentStyle={{
                          backgroundColor: '#18284F',
                          border: '1px solid #2A3D6B',
                          borderRadius: '8px',
                        }}
                        labelStyle={{ color: '#9CA3AF' }}
                        formatter={(value, name) => [value, name === 'active' ? 'Active' : 'Total']}
                      />
                      <Bar dataKey="total" fill="#2A3D6B" name="total" radius={[0, 4, 4, 0]} />
                      <Bar dataKey="active" fill="#6EDBE0" name="active" radius={[0, 4, 4, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              )}
            </CardContent>
          </Card>

          {/* Tier Breakdown */}
          <Card>
            <CardTitle>Monitoring Tiers</CardTitle>
            <CardContent className="mt-4">
              {tiers.length === 0 ? (
                <div className="text-center py-8 text-theme-muted">
                  <p>No tiers configured</p>
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Tier</TableHead>
                      <TableHead>Interval</TableHead>
                      <TableHead className="text-center">Health</TableHead>
                      <TableHead className="text-right">Targets</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {tierBreakdown.map((tier) => (
                      <TableRow key={tier.tier}>
                        <TableCell>
                          <span className="font-medium capitalize">{tier.displayName || tier.tier}</span>
                        </TableCell>
                        <TableCell className="text-theme-muted">
                          {tier.interval}
                        </TableCell>
                        <TableCell className="text-center">
                          <div className="flex items-center justify-center gap-2">
                            {tier.down > 0 && (
                              <span className="flex items-center gap-1 text-xs text-pilot-red">
                                <TrendingDown className="w-3 h-3" />
                                {tier.down}
                              </span>
                            )}
                            {tier.healthy > 0 && (
                              <span className="flex items-center gap-1 text-xs text-status-healthy">
                                <TrendingUp className="w-3 h-3" />
                                {tier.healthy}
                              </span>
                            )}
                          </div>
                        </TableCell>
                        <TableCell className="text-right text-theme-primary">
                          {tier.count.toLocaleString()}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </div>

        {/* Recent Incidents & Targets */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 md:gap-6">
          {/* Recent Incidents */}
          <Card>
            <div className="flex items-center justify-between mb-4">
              <CardTitle>Recent Incidents</CardTitle>
              <a
                href="/incidents"
                className="text-sm text-pilot-cyan hover:text-pilot-cyan-light transition-colors"
              >
                View all
              </a>
            </div>
            <CardContent>
              {incidents.length === 0 ? (
                <div className="text-center py-8 text-theme-muted">
                  <AlertCircle className="w-12 h-12 mx-auto mb-3 opacity-50" />
                  <p>No active incidents</p>
                  <p className="text-sm mt-1">All systems operational</p>
                </div>
              ) : (
                <div className="space-y-3">
                  {incidents.slice(0, 5).map(incident => (
                    <div
                      key={incident.id}
                      className="flex items-start gap-3 p-3 bg-surface-primary rounded-lg cursor-pointer hover:bg-surface-tertiary transition-colors"
                      onClick={() => navigate('/incidents')}
                    >
                      <AlertCircle className={`w-5 h-5 flex-shrink-0 ${
                        incident.severity === 'critical' ? 'text-pilot-red' :
                        incident.severity === 'major' ? 'text-warning' :
                        'text-accent'
                      }`} />
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-medium text-theme-primary truncate">
                            {incident.title || `${incident.incident_type} on ${incident.target_ip || 'multiple targets'}`}
                          </span>
                          <StatusBadge
                            status={incident.status === 'active' ? 'down' : 'degraded'}
                            label={incident.status}
                            size="sm"
                          />
                        </div>
                        <p className="text-sm text-theme-muted truncate">
                          {incident.blast_radius} • Started {formatRelativeTime(incident.started_at)}
                        </p>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>

          {/* Recent Targets */}
          <Card>
            <div className="flex items-center justify-between mb-4">
              <CardTitle>Targets Overview</CardTitle>
              <a
                href="/targets"
                className="text-sm text-pilot-cyan hover:text-pilot-cyan-light transition-colors"
              >
                View all
              </a>
            </div>
            <CardContent>
              {targets.length === 0 ? (
                <div className="text-center py-8 text-theme-muted">
                  <Target className="w-12 h-12 mx-auto mb-3 opacity-50" />
                  <p>No targets configured</p>
                  <p className="text-sm mt-1">Add targets to begin monitoring</p>
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>IP Address</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead>Tier</TableHead>
                      <TableHead className="text-right">Latency</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {targets.slice(0, 6).map((target) => {
                      const status = targetStatuses[target.id];
                      return (
                        <TableRow key={target.id}>
                          <TableCell className="font-mono">
                            <div className="flex items-center gap-2">
                              <StatusDot
                                status={status?.status === 'healthy' ? 'healthy' : status?.status === 'degraded' ? 'degraded' : status?.status === 'down' ? 'down' : 'unknown'}
                                pulse={status?.status === 'down'}
                                size="sm"
                              />
                              {target.ip}
                            </div>
                          </TableCell>
                          <TableCell>
                            <StatusBadge
                              status={status?.status || 'unknown'}
                              label={status?.status || 'unknown'}
                              size="sm"
                            />
                          </TableCell>
                          <TableCell>
                            <span className="capitalize text-theme-muted">{target.tier}</span>
                          </TableCell>
                          <TableCell className="text-right font-mono text-theme-muted">
                            {status?.avg_latency_ms != null ? `${status.avg_latency_ms.toFixed(1)}ms` : '—'}
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </div>
      </PageContent>
    </>
  );
}
