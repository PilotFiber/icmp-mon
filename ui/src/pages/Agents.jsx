import { useState, useEffect, useMemo } from 'react';
import {
  Server,
  MapPin,
  RefreshCw,
  ChevronRight,
  AlertTriangle,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card, CardTitle, CardContent } from '../components/Card';
import { MetricCard, MetricCardCompact } from '../components/MetricCard';
import { StatusBadge, StatusDot } from '../components/StatusBadge';
import { Button } from '../components/Button';
import { SearchInput, Select } from '../components/Input';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '../components/Table';
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
  const [agents, setAgents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [search, setSearch] = useState('');
  const [regionFilter, setRegionFilter] = useState('');
  const [providerFilter, setProviderFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [selectedAgent, setSelectedAgent] = useState(null);

  const fetchAgents = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await endpoints.listAgents();
      setAgents(res.agents || []);
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
                <h3 className="font-medium text-white">Failed to load agents</h3>
                <p className="text-sm text-gray-400">{error}</p>
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
          <Button variant="secondary" onClick={fetchAgents} className="gap-2">
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </Button>
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
            <div className="text-center py-12 text-gray-400">
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
                    onClick={() => setSelectedAgent(agent)}
                    className="cursor-pointer"
                  >
                    <TableCell>
                      <div className="flex items-center gap-3">
                        <div className="p-2 bg-pilot-navy-light rounded-lg">
                          <Server className="w-4 h-4 text-pilot-cyan" />
                        </div>
                        <div>
                          <div className="font-medium text-white">{agent.name}</div>
                          <div className="text-xs text-gray-500">{agent.provider}</div>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <MapPin className="w-4 h-4 text-gray-500" />
                        <div>
                          <div className="text-sm text-white">{agent.region || 'Unknown'}</div>
                          <div className="text-xs text-gray-500">{agent.location || ''}</div>
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
                    <TableCell className="text-gray-400">
                      {agent.version || 'unknown'}
                    </TableCell>
                    <TableCell className="font-mono text-sm text-gray-400">
                      {agent.public_ip || '—'}
                    </TableCell>
                    <TableCell className="text-gray-400 text-sm">
                      {agent.last_heartbeat ? formatRelativeTime(agent.last_heartbeat) : 'Never'}
                    </TableCell>
                    <TableCell>
                      <ChevronRight className="w-4 h-4 text-gray-500" />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Card>

        {/* Agent Detail Panel */}
        {selectedAgent && (
          <Card className="mt-6" accent="cyan">
            <div className="flex items-start justify-between mb-6">
              <div>
                <h3 className="text-xl font-semibold text-white">{selectedAgent.name}</h3>
                <p className="text-gray-400">
                  {selectedAgent.location} • {selectedAgent.provider}
                </p>
              </div>
              <StatusBadge
                status={selectedAgent.status === 'active' ? 'healthy' : selectedAgent.status === 'degraded' ? 'degraded' : 'down'}
                label={selectedAgent.status}
              />
            </div>

            <div className="grid grid-cols-2 md:grid-cols-5 gap-6">
              <MetricCardCompact title="Region" value={selectedAgent.region || 'Unknown'} />
              <MetricCardCompact title="Provider" value={selectedAgent.provider || 'Unknown'} />
              <MetricCardCompact title="Version" value={selectedAgent.version || 'unknown'} />
              <MetricCardCompact title="Max Targets" value={selectedAgent.max_targets?.toLocaleString() || '—'} />
              <MetricCardCompact title="Public IP" value={selectedAgent.public_ip || '—'} />
            </div>

            {selectedAgent.executors && selectedAgent.executors.length > 0 && (
              <div className="mt-6 pt-6 border-t border-pilot-navy-light">
                <h4 className="text-sm font-medium text-gray-400 mb-3">Executors</h4>
                <div className="flex flex-wrap gap-2">
                  {selectedAgent.executors.map((executor) => (
                    <span
                      key={executor}
                      className="px-3 py-1 rounded-full text-sm bg-pilot-navy-light text-pilot-cyan"
                    >
                      {executor}
                    </span>
                  ))}
                </div>
              </div>
            )}

            <div className="mt-6 flex gap-3">
              <Button variant="ghost" size="sm" onClick={() => setSelectedAgent(null)}>
                Close
              </Button>
            </div>
          </Card>
        )}
      </PageContent>
    </>
  );
}
