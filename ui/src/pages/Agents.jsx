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
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card, CardTitle, CardContent } from '../components/Card';
import { MetricCard } from '../components/MetricCard';
import { StatusBadge } from '../components/StatusBadge';
import { Button } from '../components/Button';
import { SearchInput, Select } from '../components/Input';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '../components/Table';
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

  useEffect(() => {
    fetchAgents();
    const interval = setInterval(fetchAgents, 10000);
    return () => clearInterval(interval);
  }, []);

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

  const filteredAgents = useMemo(() => {
    return agents.filter((agent) => {
      if (search && !agent.name.toLowerCase().includes(search.toLowerCase())) {
        return false;
      }
      if (regionFilter && agent.region !== regionFilter) {
        return false;
      }
      if (providerFilter && agent.provider !== providerFilter) {
        return false;
      }
      if (statusFilter && agent.status !== statusFilter) {
        return false;
      }
      return true;
    });
  }, [agents, search, regionFilter, providerFilter, statusFilter]);

  const stats = useMemo(() => {
    const total = agents.length;
    const active = agents.filter((a) => a.status === 'active').length;
    const degraded = agents.filter((a) => a.status === 'degraded').length;
    const offline = agents.filter((a) => a.status === 'offline').length;

    return { total, active, degraded, offline };
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
          <div className="flex gap-3">
            <Button variant="secondary" onClick={fetchAgents} className="gap-2">
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </Button>
            <Button onClick={() => setShowEnrollModal(true)} className="gap-2">
              <Plus className="w-4 h-4" />
              Enroll Agent
            </Button>
          </div>
        }
      />

      <PageContent>
        {/* Summary Cards */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
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
          <Card className="mb-6">
            <CardTitle icon={Activity}>Fleet Operations</CardTitle>
            <CardContent>
              <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-4">
                <div className="bg-surface-primary rounded-lg p-4">
                  <div className="flex items-center gap-2 text-theme-muted text-xs mb-1">
                    <Target className="w-3 h-3" />
                    Total Targets
                  </div>
                  <div className="text-2xl font-semibold text-theme-primary">
                    {fleetOverview.total_targets?.toLocaleString() || 0}
                  </div>
                  <div className="text-xs text-theme-muted mt-1">
                    {fleetOverview.total_active_targets?.toLocaleString() || 0} active
                  </div>
                </div>
                <div className="bg-surface-primary rounded-lg p-4">
                  <div className="flex items-center gap-2 text-theme-muted text-xs mb-1">
                    <Zap className="w-3 h-3" />
                    Probes/sec
                  </div>
                  <div className="text-2xl font-semibold text-theme-primary">
                    {fleetOverview.total_probes_per_second?.toFixed(1) || 0}
                  </div>
                  <div className="text-xs text-theme-muted mt-1">
                    fleet-wide
                  </div>
                </div>
                <div className="bg-surface-primary rounded-lg p-4">
                  <div className="flex items-center gap-2 text-theme-muted text-xs mb-1">
                    <Database className="w-3 h-3" />
                    Results Queued
                  </div>
                  <div className="text-2xl font-semibold text-theme-primary">
                    {fleetOverview.total_results_queued?.toLocaleString() || 0}
                  </div>
                  <div className="text-xs text-theme-muted mt-1">
                    pending delivery
                  </div>
                </div>
                <div className="bg-surface-primary rounded-lg p-4">
                  <div className="flex items-center gap-2 text-theme-muted text-xs mb-1">
                    <Cpu className="w-3 h-3" />
                    Avg CPU
                  </div>
                  <div className="text-2xl font-semibold text-theme-primary">
                    {fleetOverview.avg_cpu_percent?.toFixed(1) || 0}%
                  </div>
                  <div className="text-xs text-theme-muted mt-1">
                    across agents
                  </div>
                </div>
                <div className="bg-surface-primary rounded-lg p-4">
                  <div className="flex items-center gap-2 text-theme-muted text-xs mb-1">
                    <HardDrive className="w-3 h-3" />
                    Avg Memory
                  </div>
                  <div className="text-2xl font-semibold text-theme-primary">
                    {fleetOverview.avg_memory_mb?.toFixed(1) || 0} MB
                  </div>
                  <div className="text-xs text-theme-muted mt-1">
                    per agent
                  </div>
                </div>
                <div className="bg-surface-primary rounded-lg p-4">
                  <div className="flex items-center gap-2 text-theme-muted text-xs mb-1">
                    <Server className="w-3 h-3" />
                    Active Agents
                  </div>
                  <div className="text-2xl font-semibold text-pilot-cyan">
                    {fleetOverview.active_agents || 0}
                  </div>
                  <div className="text-xs text-theme-muted mt-1">
                    of {fleetOverview.total_agents || 0} total
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        )}

        {/* Filters */}
        <Card className="mb-6">
          <div className="flex flex-wrap gap-4">
            <SearchInput
              value={search}
              onChange={setSearch}
              placeholder="Search agents..."
              className="w-64"
            />
            <Select
              options={regionOptions}
              value={regionFilter}
              onChange={setRegionFilter}
              className="w-40"
            />
            <Select
              options={providerOptions}
              value={providerFilter}
              onChange={setProviderFilter}
              className="w-40"
            />
            <Select
              options={statuses}
              value={statusFilter}
              onChange={setStatusFilter}
              className="w-40"
            />
          </div>
        </Card>

        {/* Agent List */}
        <Card>
          {filteredAgents.length === 0 ? (
            <div className="text-center py-12 text-theme-muted">
              <Server className="w-12 h-12 mx-auto mb-4 opacity-50" />
              {agents.length === 0 ? (
                <>
                  <p>No agents registered</p>
                  <p className="text-sm mt-1">Start an agent to begin monitoring</p>
                </>
              ) : (
                <p>No agents match your filters</p>
              )}
            </div>
          ) : (
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
                      <StatusBadge
                        status={agent.status === 'active' ? 'healthy' : agent.status === 'degraded' ? 'degraded' : 'down'}
                        label={agent.status}
                        pulse={agent.status === 'offline'}
                        size="sm"
                      />
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
