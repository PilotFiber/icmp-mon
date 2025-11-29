import { useState, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Server,
  MapPin,
  RefreshCw,
  ChevronRight,
  AlertTriangle,
  Plus,
  Target,
  Zap,
  Database,
  Cpu,
  HardDrive,
  Activity,
  Shuffle,
  CheckCircle,
  XCircle,
  Loader2,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card, CardTitle, CardContent } from '../components/Card';
import { MetricCard } from '../components/MetricCard';
import { StatusBadge } from '../components/StatusBadge';
import { Button } from '../components/Button';
import { SearchInput, Select } from '../components/Input';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell, MobileCardList, MobileCard, MobileCardRow } from '../components/Table';
import { EnrollAgentModal } from '../components/EnrollAgentModal';
import { formatRelativeTime } from '../lib/utils';
import { endpoints } from '../lib/api';

const regions = [
  { value: '', label: 'All Regions' },
];

const providers = [
  { value: '', label: 'All Providers' },
];

const statuses = [
  { value: '', label: 'All Statuses' },
  { value: 'active', label: 'Active' },
  { value: 'degraded', label: 'Degraded' },
  { value: 'offline', label: 'Offline' },
  { value: 'archived', label: 'Archived' },
];

export function Agents() {
  const navigate = useNavigate();
  const [agents, setAgents] = useState([]);
  const [fleetOverview, setFleetOverview] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [search, setSearch] = useState('');
  const [regionFilter, setRegionFilter] = useState('');
  const [providerFilter, setProviderFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [showEnrollModal, setShowEnrollModal] = useState(false);
  const [rebalanceStatus, setRebalanceStatus] = useState(null);
  const [isRebalancing, setIsRebalancing] = useState(false);

  const fetchAgents = async () => {
    try {
      setLoading(true);
      setError(null);
      const [agentsRes, overviewRes] = await Promise.all([
        endpoints.listAgents(),
        endpoints.getFleetOverview().catch(() => null),
      ]);
      setAgents(agentsRes.agents || []);
      setFleetOverview(overviewRes);
    } catch (err) {
      console.error('Failed to fetch agents:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const fetchRebalanceStatus = async () => {
    try {
      const status = await endpoints.getAssignmentStatus();
      setRebalanceStatus(status);
      setIsRebalancing(status.is_running);
    } catch (err) {
      console.error('Failed to fetch rebalance status:', err);
    }
  };

  const handleRebalance = async () => {
    if (isRebalancing) return;

    try {
      setIsRebalancing(true);
      await endpoints.triggerRebalance();
      // Poll for completion
      const pollStatus = setInterval(async () => {
        const status = await endpoints.getAssignmentStatus();
        setRebalanceStatus(status);
        if (!status.is_running) {
          setIsRebalancing(false);
          clearInterval(pollStatus);
          fetchAgents(); // Refresh agents after rebalance
        }
      }, 2000);
    } catch (err) {
      console.error('Failed to trigger rebalance:', err);
      setIsRebalancing(false);
    }
  };

  useEffect(() => {
    fetchAgents();
    fetchRebalanceStatus();
    // Pause auto-refresh when modal is open
    if (showEnrollModal) return;
    const interval = setInterval(fetchAgents, 10000);
    return () => clearInterval(interval);
  }, [showEnrollModal]);

  // Build dynamic filter options from data
  const regionOptions = useMemo(() => {
    const uniqueRegions = [...new Set(agents.map(a => a.region).filter(Boolean))];
    return [
      { value: '', label: 'All Regions' },
      ...uniqueRegions.map(r => ({ value: r, label: r })),
    ];
  }, [agents]);

  const providerOptions = useMemo(() => {
    const uniqueProviders = [...new Set(agents.map(a => a.provider).filter(Boolean))];
    return [
      { value: '', label: 'All Providers' },
      ...uniqueProviders.map(p => ({ value: p, label: p })),
    ];
  }, [agents]);

  // Determine if an agent is archived
  const isArchived = (agent) => !!agent.archived_at;

  // Get the effective status of an agent (archived takes precedence)
  const getEffectiveStatus = (agent) => isArchived(agent) ? 'archived' : agent.status;

  const filteredAgents = useMemo(() => {
    return agents.filter((agent) => {
      const effectiveStatus = getEffectiveStatus(agent);

      // By default (no status filter), hide archived agents
      if (!statusFilter && effectiveStatus === 'archived') {
        return false;
      }

      if (search && !agent.name.toLowerCase().includes(search.toLowerCase())) {
        return false;
      }
      if (regionFilter && agent.region !== regionFilter) {
        return false;
      }
      if (providerFilter && agent.provider !== providerFilter) {
        return false;
      }
      if (statusFilter && effectiveStatus !== statusFilter) {
        return false;
      }
      return true;
    });
  }, [agents, search, regionFilter, providerFilter, statusFilter]);

  const stats = useMemo(() => {
    // Filter out archived agents from the main counts
    const nonArchived = agents.filter((a) => !isArchived(a));
    const total = nonArchived.length;
    const active = nonArchived.filter((a) => a.status === 'active').length;
    const degraded = nonArchived.filter((a) => a.status === 'degraded').length;
    const offline = nonArchived.filter((a) => a.status === 'offline').length;
    const archived = agents.filter((a) => isArchived(a)).length;

    return { total, active, degraded, offline, archived };
  }, [agents]);

  if (error) {
    return (
      <>
        <PageHeader title="Agents" />
        <PageContent>
          <Card accent="red">
            <div className="flex items-center gap-3">
              <AlertTriangle className="w-6 h-6 text-pilot-red" />
              <div>
                <h3 className="font-medium text-theme-primary">Failed to load agents</h3>
                <p className="text-sm text-theme-muted">{error}</p>
              </div>
              <Button variant="secondary" size="sm" onClick={fetchAgents} className="ml-auto">
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
        title="Agents"
        description={`${stats.total} agents registered`}
        actions={
          <div className="flex gap-2 md:gap-3">
            <Button variant="secondary" onClick={fetchAgents} className="gap-2" size="sm">
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              <span className="hidden sm:inline">Refresh</span>
            </Button>
            <Button onClick={() => setShowEnrollModal(true)} className="gap-2" size="sm">
              <Plus className="w-4 h-4" />
              <span className="hidden sm:inline">Enroll Agent</span>
            </Button>
          </div>
        }
      />

      <PageContent>
        {/* Summary Cards */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 md:gap-4 mb-4 md:mb-6">
          <MetricCard
            title="Total Agents"
            value={stats.total}
            icon={Server}
          />
          <MetricCard
            title="Active"
            value={stats.active}
            status="healthy"
          />
          <MetricCard
            title="Degraded"
            value={stats.degraded}
            status={stats.degraded > 0 ? 'degraded' : 'healthy'}
          />
          <MetricCard
            title="Offline"
            value={stats.offline}
            status={stats.offline > 0 ? 'down' : 'healthy'}
          />
        </div>

        {/* Fleet Overview */}
        {fleetOverview && (
          <Card className="mb-4 md:mb-6">
            <CardTitle icon={Activity}>Fleet Operations</CardTitle>
            <CardContent>
              <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-2 sm:gap-3 md:gap-4">
                <div className="bg-surface-primary rounded-lg p-2.5 sm:p-3 md:p-4">
                  <div className="flex items-center gap-1.5 sm:gap-2 text-theme-muted text-[10px] sm:text-xs mb-0.5 sm:mb-1">
                    <Target className="w-3 h-3" />
                    <span className="truncate">Total Targets</span>
                  </div>
                  <div className="text-lg sm:text-xl md:text-2xl font-semibold text-theme-primary">
                    {fleetOverview.total_targets?.toLocaleString() || 0}
                  </div>
                  <div className="text-[10px] sm:text-xs text-theme-muted mt-0.5 sm:mt-1 truncate">
                    {fleetOverview.total_active_targets?.toLocaleString() || 0} active
                  </div>
                </div>
                <div className="bg-surface-primary rounded-lg p-2.5 sm:p-3 md:p-4">
                  <div className="flex items-center gap-1.5 sm:gap-2 text-theme-muted text-[10px] sm:text-xs mb-0.5 sm:mb-1">
                    <Zap className="w-3 h-3" />
                    <span className="truncate">Probes/sec</span>
                  </div>
                  <div className="text-lg sm:text-xl md:text-2xl font-semibold text-theme-primary">
                    {fleetOverview.total_probes_per_second?.toFixed(1) || 0}
                  </div>
                  <div className="text-[10px] sm:text-xs text-theme-muted mt-0.5 sm:mt-1">
                    fleet-wide
                  </div>
                </div>
                <div className="bg-surface-primary rounded-lg p-2.5 sm:p-3 md:p-4">
                  <div className="flex items-center gap-1.5 sm:gap-2 text-theme-muted text-[10px] sm:text-xs mb-0.5 sm:mb-1">
                    <Database className="w-3 h-3" />
                    <span className="truncate">Queued</span>
                  </div>
                  <div className="text-lg sm:text-xl md:text-2xl font-semibold text-theme-primary">
                    {fleetOverview.total_results_queued?.toLocaleString() || 0}
                  </div>
                  <div className="text-[10px] sm:text-xs text-theme-muted mt-0.5 sm:mt-1 truncate">
                    pending
                  </div>
                </div>
                <div className="bg-surface-primary rounded-lg p-2.5 sm:p-3 md:p-4">
                  <div className="flex items-center gap-1.5 sm:gap-2 text-theme-muted text-[10px] sm:text-xs mb-0.5 sm:mb-1">
                    <Cpu className="w-3 h-3" />
                    <span className="truncate">Avg CPU</span>
                  </div>
                  <div className="text-lg sm:text-xl md:text-2xl font-semibold text-theme-primary">
                    {fleetOverview.avg_cpu_percent?.toFixed(1) || 0}%
                  </div>
                  <div className="text-[10px] sm:text-xs text-theme-muted mt-0.5 sm:mt-1 truncate">
                    across agents
                  </div>
                </div>
                <div className="bg-surface-primary rounded-lg p-2.5 sm:p-3 md:p-4">
                  <div className="flex items-center gap-1.5 sm:gap-2 text-theme-muted text-[10px] sm:text-xs mb-0.5 sm:mb-1">
                    <HardDrive className="w-3 h-3" />
                    <span className="truncate">Avg Memory</span>
                  </div>
                  <div className="text-lg sm:text-xl md:text-2xl font-semibold text-theme-primary">
                    {fleetOverview.avg_memory_mb?.toFixed(0) || 0}<span className="text-sm sm:text-base">MB</span>
                  </div>
                  <div className="text-[10px] sm:text-xs text-theme-muted mt-0.5 sm:mt-1">
                    per agent
                  </div>
                </div>
                <div className="bg-surface-primary rounded-lg p-2.5 sm:p-3 md:p-4">
                  <div className="flex items-center gap-1.5 sm:gap-2 text-theme-muted text-[10px] sm:text-xs mb-0.5 sm:mb-1">
                    <Server className="w-3 h-3" />
                    <span className="truncate">Active</span>
                  </div>
                  <div className="text-lg sm:text-xl md:text-2xl font-semibold text-pilot-cyan">
                    {fleetOverview.active_agents || 0}
                  </div>
                  <div className="text-[10px] sm:text-xs text-theme-muted mt-0.5 sm:mt-1 truncate">
                    of {fleetOverview.total_agents || 0} total
                  </div>
                </div>
              </div>

              {/* Rebalance Button */}
              <div className="mt-3 sm:mt-4 pt-3 sm:pt-4 border-t border-border-subtle flex flex-col sm:flex-row sm:items-center gap-3 sm:justify-between">
                <div className="flex items-center gap-2 sm:gap-3">
                  <Shuffle className="w-4 h-4 text-theme-muted flex-shrink-0" />
                  <div className="min-w-0">
                    <div className="text-sm text-theme-primary font-medium">Target Assignment</div>
                    <div className="text-xs text-theme-muted truncate">
                      {rebalanceStatus?.last_completed ? (
                        <>
                          <span className="hidden sm:inline">Last rebalanced: </span>
                          {new Date(rebalanceStatus.last_completed).toLocaleString()}
                          {rebalanceStatus.last_assignments > 0 && (
                            <span className="hidden md:inline"> ({rebalanceStatus.last_assignments.toLocaleString()} assignments)</span>
                          )}
                          {rebalanceStatus.last_error && (
                            <span className="text-pilot-red ml-2">Error</span>
                          )}
                        </>
                      ) : (
                        'Distribute targets across agents'
                      )}
                    </div>
                  </div>
                </div>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={handleRebalance}
                  disabled={isRebalancing}
                  className="gap-2 w-full sm:w-auto"
                >
                  {isRebalancing ? (
                    <>
                      <Loader2 className="w-4 h-4 animate-spin" />
                      <span className="sm:hidden">Rebalancing</span>
                      <span className="hidden sm:inline">Rebalancing...</span>
                    </>
                  ) : (
                    <>
                      <Shuffle className="w-4 h-4" />
                      <span className="sm:hidden">Rebalance</span>
                      <span className="hidden sm:inline">Rebalance Targets</span>
                    </>
                  )}
                </Button>
              </div>
            </CardContent>
          </Card>
        )}

        {/* Filters */}
        <Card className="mb-4 md:mb-6">
          <div className="flex flex-col sm:flex-row sm:flex-wrap gap-2 sm:gap-3 md:gap-4">
            <SearchInput
              value={search}
              onChange={setSearch}
              placeholder="Search agents..."
              className="w-full sm:w-64"
            />
            <div className="grid grid-cols-3 sm:flex gap-2 sm:gap-3 md:gap-4">
              <Select
                options={regionOptions}
                value={regionFilter}
                onChange={setRegionFilter}
                className="w-full sm:w-32 md:w-40"
              />
              <Select
                options={providerOptions}
                value={providerFilter}
                onChange={setProviderFilter}
                className="w-full sm:w-32 md:w-40"
              />
              <Select
                options={statuses}
                value={statusFilter}
                onChange={setStatusFilter}
                className="w-full sm:w-32 md:w-40"
              />
            </div>
          </div>
        </Card>

        {/* Agent List */}
        <Card>
          {filteredAgents.length === 0 ? (
            <div className="text-center py-8 md:py-12 text-theme-muted">
              <Server className="w-10 h-10 md:w-12 md:h-12 mx-auto mb-3 md:mb-4 opacity-50" />
              {agents.length === 0 ? (
                <>
                  <p className="text-sm md:text-base">No agents registered</p>
                  <p className="text-xs md:text-sm mt-1">Start an agent to begin monitoring</p>
                </>
              ) : (
                <p className="text-sm md:text-base">No agents match your filters</p>
              )}
            </div>
          ) : (
            <>
              {/* Mobile Card View */}
              <MobileCardList>
                {filteredAgents.map((agent) => {
                  const effectiveStatus = getEffectiveStatus(agent);
                  return (
                    <MobileCard
                      key={agent.id}
                      onClick={() => navigate(`/agents/${agent.id}`)}
                    >
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2 min-w-0">
                          <div className="p-1.5 bg-surface-tertiary rounded-lg flex-shrink-0">
                            <Server className="w-4 h-4 text-pilot-cyan" />
                          </div>
                          <div className="min-w-0">
                            <div className="font-medium text-theme-primary text-sm truncate">{agent.name}</div>
                            <div className="text-xs text-theme-muted">{agent.provider}</div>
                          </div>
                        </div>
                        <div className="flex items-center gap-2 flex-shrink-0">
                          <StatusBadge
                            status={effectiveStatus === 'active' ? 'healthy' : effectiveStatus === 'degraded' ? 'degraded' : effectiveStatus === 'archived' ? 'unknown' : 'down'}
                            label={effectiveStatus}
                            pulse={effectiveStatus === 'offline'}
                            size="sm"
                          />
                          <ChevronRight className="w-4 h-4 text-theme-muted" />
                        </div>
                      </div>
                      <div className="grid grid-cols-2 gap-2 pt-2 border-t border-border-subtle text-xs">
                        <div>
                          <span className="text-theme-muted">Region:</span>{' '}
                          <span className="text-theme-primary">{agent.region || 'Unknown'}</span>
                        </div>
                        <div>
                          <span className="text-theme-muted">Version:</span>{' '}
                          <span className="text-theme-primary">{agent.version || 'unknown'}</span>
                        </div>
                        <div className="col-span-2">
                          <span className="text-theme-muted">Last seen:</span>{' '}
                          <span className="text-theme-primary">{agent.last_heartbeat ? formatRelativeTime(agent.last_heartbeat) : 'Never'}</span>
                        </div>
                      </div>
                    </MobileCard>
                  );
                })}
              </MobileCardList>

              {/* Desktop Table View */}
              <div className="hidden md:block">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Agent</TableHead>
                      <TableHead>Region / Location</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead>Version</TableHead>
                      <TableHead>Public IP</TableHead>
                      <TableHead>Last Heartbeat</TableHead>
                      <TableHead></TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredAgents.map((agent) => (
                      <TableRow
                        key={agent.id}
                        onClick={() => navigate(`/agents/${agent.id}`)}
                        className="cursor-pointer"
                      >
                        <TableCell>
                          <div className="flex items-center gap-3">
                            <div className="p-2 bg-surface-tertiary rounded-lg">
                              <Server className="w-4 h-4 text-pilot-cyan" />
                            </div>
                            <div>
                              <div className="font-medium text-theme-primary">{agent.name}</div>
                              <div className="text-xs text-theme-muted">{agent.provider}</div>
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <MapPin className="w-4 h-4 text-theme-muted" />
                            <div>
                              <div className="text-sm text-theme-primary">{agent.region || 'Unknown'}</div>
                              <div className="text-xs text-theme-muted">{agent.location || ''}</div>
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          {(() => {
                            const effectiveStatus = getEffectiveStatus(agent);
                            return (
                              <StatusBadge
                                status={effectiveStatus === 'active' ? 'healthy' : effectiveStatus === 'degraded' ? 'degraded' : effectiveStatus === 'archived' ? 'unknown' : 'down'}
                                label={effectiveStatus}
                                pulse={effectiveStatus === 'offline'}
                                size="sm"
                              />
                            );
                          })()}
                        </TableCell>
                        <TableCell className="text-theme-muted">
                          {agent.version || 'unknown'}
                        </TableCell>
                        <TableCell className="font-mono text-sm text-theme-muted">
                          {agent.public_ip || '—'}
                        </TableCell>
                        <TableCell className="text-theme-muted text-sm">
                          {agent.last_heartbeat ? formatRelativeTime(agent.last_heartbeat) : 'Never'}
                        </TableCell>
                        <TableCell>
                          <ChevronRight className="w-4 h-4 text-theme-muted" />
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </>
          )}
        </Card>

      </PageContent>

      {/* Enroll Agent Modal */}
      <EnrollAgentModal
        isOpen={showEnrollModal}
        onClose={() => setShowEnrollModal(false)}
        onSuccess={() => {
          setShowEnrollModal(false);
          fetchAgents();
        }}
      />
    </>
  );
}
