import { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Server,
  MapPin,
  RefreshCw,
  AlertTriangle,
  Cpu,
  HardDrive,
  Activity,
  Clock,
  Globe,
  Shield,
  BarChart3,
  Database,
  Zap,
  Target,
  Edit2,
} from 'lucide-react';
import {
  LineChart,
  Line,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  CartesianGrid,
} from 'recharts';

import { PageHeader, PageContent } from '../components/Layout';
import { Card } from '../components/Card';
import { MetricCardCompact } from '../components/MetricCard';
import { StatusBadge } from '../components/StatusBadge';
import { Button } from '../components/Button';
import { Modal } from '../components/Modal';
import { Input } from '../components/Input';
import { formatRelativeTime } from '../lib/utils';
import { endpoints } from '../lib/api';

const TIME_WINDOWS = [
  { label: '1h', value: '1h' },
  { label: '6h', value: '6h' },
  { label: '24h', value: '24h' },
];

export function AgentDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [agent, setAgent] = useState(null);
  const [stats, setStats] = useState(null);
  const [metrics, setMetrics] = useState([]);
  const [timeWindow, setTimeWindow] = useState('1h');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [showEditModal, setShowEditModal] = useState(false);

  const fetchAgent = async () => {
    try {
      setLoading(true);
      setError(null);
      const [agentRes, statsRes] = await Promise.all([
        endpoints.getAgent(id),
        endpoints.getAgentStats(id).catch(() => null),
      ]);
      setAgent(agentRes);
      setStats(statsRes);
    } catch (err) {
      console.error('Failed to fetch agent:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const fetchMetrics = async () => {
    try {
      const res = await endpoints.getAgentMetrics(id, timeWindow);
      setMetrics(res.metrics || []);
    } catch (err) {
      console.error('Failed to fetch metrics:', err);
    }
  };

  useEffect(() => {
    fetchAgent();
    const interval = setInterval(fetchAgent, 10000);
    return () => clearInterval(interval);
  }, [id]);

  useEffect(() => {
    fetchMetrics();
    const interval = setInterval(fetchMetrics, 30000);
    return () => clearInterval(interval);
  }, [id, timeWindow]);

  const getStatusColor = (status) => {
    switch (status) {
      case 'active': return 'healthy';
      case 'degraded': return 'degraded';
      case 'offline': return 'down';
      default: return 'unknown';
    }
  };

  if (loading) {
    return (
      <>
        <PageHeader
          title="Loading..."
          breadcrumbs={[
            { label: 'Agents', href: '/agents' },
            { label: '...' },
          ]}
        />
        <PageContent>
          <div className="flex items-center justify-center py-12">
            <RefreshCw className="w-6 h-6 animate-spin text-theme-muted" />
          </div>
        </PageContent>
      </>
    );
  }

  if (error || !agent) {
    return (
      <>
        <PageHeader
          title="Agent Not Found"
          breadcrumbs={[
            { label: 'Agents', href: '/agents' },
            { label: 'Error' },
          ]}
        />
        <PageContent>
          <Card accent="red">
            <div className="flex items-center gap-3">
              <AlertTriangle className="w-6 h-6 text-pilot-red" />
              <div>
                <h3 className="font-medium text-theme-primary">Failed to load agent</h3>
                <p className="text-sm text-theme-muted">{error || 'Agent not found'}</p>
              </div>
              <Button variant="secondary" size="sm" onClick={() => navigate('/agents')} className="ml-auto">
                Back to Agents
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
        title={agent.name}
        breadcrumbs={[
          { label: 'Agents', href: '/agents' },
          { label: agent.name },
        ]}
        actions={
          <div className="flex gap-3">
            <Button variant="secondary" onClick={() => setShowEditModal(true)} className="gap-2">
              <Edit2 className="w-4 h-4" />
              Edit
            </Button>
            <Button variant="secondary" onClick={fetchAgent} className="gap-2">
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </Button>
          </div>
        }
      />

      <PageContent>
        {/* Main Info Card */}
        <Card accent={agent.status === 'active' ? 'cyan' : agent.status === 'degraded' ? 'warning' : 'red'} className="mb-6">
          <div className="flex items-start justify-between mb-6">
            <div>
              <div className="flex items-center gap-3 mb-1">
                <div className="p-2 bg-surface-tertiary rounded-lg">
                  <Server className="w-6 h-6 text-pilot-cyan" />
                </div>
                <div>
                  <h2 className="text-xl font-semibold text-theme-primary">{agent.name}</h2>
                  <p className="text-theme-muted flex items-center gap-2">
                    <MapPin className="w-4 h-4" />
                    {agent.location || agent.region || 'Unknown location'} • {agent.provider || 'Unknown provider'}
                  </p>
                </div>
              </div>
            </div>
            <StatusBadge
              status={getStatusColor(agent.status)}
              label={agent.status}
              pulse={agent.status === 'offline'}
            />
          </div>

          {/* Key Metrics */}
          <div className="grid grid-cols-2 md:grid-cols-5 gap-4 mb-6">
            <MetricCardCompact
              title="Region"
              value={agent.region || 'Unknown'}
            />
            <MetricCardCompact
              title="Provider"
              value={agent.provider || 'Unknown'}
            />
            <MetricCardCompact
              title="Version"
              value={agent.version || 'unknown'}
            />
            <MetricCardCompact
              title="Max Targets"
              value={agent.max_targets?.toLocaleString() || '—'}
            />
            <MetricCardCompact
              title="Public IP"
              value={agent.public_ip || '—'}
            />
          </div>

          {/* Connection Info */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div className="bg-surface-primary rounded-lg p-4">
              <h4 className="text-sm font-medium text-theme-muted mb-3 flex items-center gap-2">
                <Clock className="w-4 h-4" />
                Connection Status
              </h4>
              <div className="space-y-3">
                <div className="flex justify-between">
                  <span className="text-theme-muted">Last Heartbeat</span>
                  <span className="text-theme-primary">
                    {agent.last_heartbeat ? formatRelativeTime(agent.last_heartbeat) : 'Never'}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-theme-muted">Enrolled</span>
                  <span className="text-theme-primary">
                    {agent.created_at ? formatRelativeTime(agent.created_at) : 'Unknown'}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-theme-muted">Agent ID</span>
                  <span className="text-theme-primary font-mono text-xs">{agent.id}</span>
                </div>
              </div>
            </div>

            <div className="bg-surface-primary rounded-lg p-4">
              <h4 className="text-sm font-medium text-theme-muted mb-3 flex items-center gap-2">
                <Globe className="w-4 h-4" />
                Network Info
              </h4>
              <div className="space-y-3">
                <div className="flex justify-between">
                  <span className="text-theme-muted">Public IP</span>
                  <span className="text-theme-primary font-mono">{agent.public_ip || '—'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-theme-muted">Location</span>
                  <span className="text-theme-primary">{agent.location || '—'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-theme-muted">Region</span>
                  <span className="text-theme-primary">{agent.region || '—'}</span>
                </div>
              </div>
            </div>
          </div>
        </Card>

        {/* Current Stats */}
        {stats && (
          <Card className="mb-6">
            <h3 className="text-sm font-medium text-theme-muted mb-4 flex items-center gap-2">
              <BarChart3 className="w-4 h-4" />
              Current Stats
            </h3>
            <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-4">
              <div className="bg-surface-primary rounded-lg p-3">
                <div className="flex items-center gap-2 text-theme-muted text-xs mb-1">
                  <Target className="w-3 h-3" />
                  Active Targets
                </div>
                <div className="text-xl font-semibold text-theme-primary">{stats.active_targets?.toLocaleString() || 0}</div>
              </div>
              <div className="bg-surface-primary rounded-lg p-3">
                <div className="flex items-center gap-2 text-theme-muted text-xs mb-1">
                  <Zap className="w-3 h-3" />
                  Probes/sec
                </div>
                <div className="text-xl font-semibold text-theme-primary">{stats.probes_per_second?.toFixed(1) || 0}</div>
              </div>
              <div className="bg-surface-primary rounded-lg p-3">
                <div className="flex items-center gap-2 text-theme-muted text-xs mb-1">
                  <Database className="w-3 h-3" />
                  Results Queued
                </div>
                <div className="text-xl font-semibold text-theme-primary">{stats.results_queued?.toLocaleString() || 0}</div>
              </div>
              <div className="bg-surface-primary rounded-lg p-3">
                <div className="flex items-center gap-2 text-theme-muted text-xs mb-1">
                  <Database className="w-3 h-3" />
                  Results Shipped
                </div>
                <div className="text-xl font-semibold text-theme-primary">{stats.results_shipped?.toLocaleString() || 0}</div>
              </div>
              <div className="bg-surface-primary rounded-lg p-3">
                <div className="flex items-center gap-2 text-theme-muted text-xs mb-1">
                  <Cpu className="w-3 h-3" />
                  CPU
                </div>
                <div className="text-xl font-semibold text-theme-primary">{stats.cpu_percent?.toFixed(1) || 0}%</div>
              </div>
              <div className="bg-surface-primary rounded-lg p-3">
                <div className="flex items-center gap-2 text-theme-muted text-xs mb-1">
                  <HardDrive className="w-3 h-3" />
                  Memory
                </div>
                <div className="text-xl font-semibold text-theme-primary">{stats.memory_mb?.toFixed(1) || 0} MB</div>
              </div>
            </div>
          </Card>
        )}

        {/* Metrics Charts */}
        {metrics.length > 0 && (
          <Card className="mb-6">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-medium text-theme-muted flex items-center gap-2">
                <Activity className="w-4 h-4" />
                Performance Metrics
              </h3>
              <div className="flex gap-1">
                {TIME_WINDOWS.map((tw) => (
                  <button
                    key={tw.value}
                    onClick={() => setTimeWindow(tw.value)}
                    className={`px-2 py-1 text-xs rounded transition-colors ${
                      timeWindow === tw.value
                        ? 'bg-pilot-cyan text-surface-primary'
                        : 'bg-surface-tertiary text-theme-muted hover:text-theme-primary'
                    }`}
                  >
                    {tw.label}
                  </button>
                ))}
              </div>
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
              {/* Memory Chart */}
              <div>
                <h4 className="text-xs font-medium text-theme-muted mb-2">Memory Usage (MB)</h4>
                <ResponsiveContainer width="100%" height={180}>
                  <AreaChart data={metrics.map(m => ({
                    ...m,
                    timeLabel: new Date(m.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
                  }))}>
                    <defs>
                      <linearGradient id="memoryGradient" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor="#06b6d4" stopOpacity={0.3}/>
                        <stop offset="95%" stopColor="#06b6d4" stopOpacity={0}/>
                      </linearGradient>
                    </defs>
                    <CartesianGrid strokeDasharray="3 3" stroke="#2A3D6B" />
                    <XAxis dataKey="timeLabel" stroke="#6B7280" fontSize={10} tickLine={false} axisLine={false} interval="preserveStartEnd" />
                    <YAxis stroke="#6B7280" fontSize={10} tickLine={false} axisLine={false} tickFormatter={(v) => `${v.toFixed(0)}`} />
                    <Tooltip
                      contentStyle={{ backgroundColor: '#18284F', border: '1px solid #2A3D6B', borderRadius: '8px' }}
                      labelStyle={{ color: '#9CA3AF' }}
                      formatter={(value) => [`${value?.toFixed(2)} MB`, 'Memory']}
                    />
                    <Area type="monotone" dataKey="memory_mb" stroke="#06b6d4" fill="url(#memoryGradient)" strokeWidth={2} />
                  </AreaChart>
                </ResponsiveContainer>
              </div>

              {/* Results Queued Chart */}
              <div>
                <h4 className="text-xs font-medium text-theme-muted mb-2">Results Queued</h4>
                <ResponsiveContainer width="100%" height={180}>
                  <LineChart data={metrics.map(m => ({
                    ...m,
                    timeLabel: new Date(m.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
                  }))}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#2A3D6B" />
                    <XAxis dataKey="timeLabel" stroke="#6B7280" fontSize={10} tickLine={false} axisLine={false} interval="preserveStartEnd" />
                    <YAxis stroke="#6B7280" fontSize={10} tickLine={false} axisLine={false} />
                    <Tooltip
                      contentStyle={{ backgroundColor: '#18284F', border: '1px solid #2A3D6B', borderRadius: '8px' }}
                      labelStyle={{ color: '#9CA3AF' }}
                      formatter={(value) => [value?.toLocaleString(), 'Queued']}
                    />
                    <Line type="monotone" dataKey="results_queued" stroke="#f59e0b" strokeWidth={2} dot={false} />
                  </LineChart>
                </ResponsiveContainer>
              </div>

              {/* Goroutines Chart */}
              <div>
                <h4 className="text-xs font-medium text-theme-muted mb-2">Goroutines</h4>
                <ResponsiveContainer width="100%" height={180}>
                  <LineChart data={metrics.map(m => ({
                    ...m,
                    timeLabel: new Date(m.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
                  }))}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#2A3D6B" />
                    <XAxis dataKey="timeLabel" stroke="#6B7280" fontSize={10} tickLine={false} axisLine={false} interval="preserveStartEnd" />
                    <YAxis stroke="#6B7280" fontSize={10} tickLine={false} axisLine={false} />
                    <Tooltip
                      contentStyle={{ backgroundColor: '#18284F', border: '1px solid #2A3D6B', borderRadius: '8px' }}
                      labelStyle={{ color: '#9CA3AF' }}
                      formatter={(value) => [value, 'Goroutines']}
                    />
                    <Line type="monotone" dataKey="goroutine_count" stroke="#22c55e" strokeWidth={2} dot={false} />
                  </LineChart>
                </ResponsiveContainer>
              </div>

              {/* Results Shipped Chart */}
              <div>
                <h4 className="text-xs font-medium text-theme-muted mb-2">Results Shipped (Total)</h4>
                <ResponsiveContainer width="100%" height={180}>
                  <AreaChart data={metrics.map(m => ({
                    ...m,
                    timeLabel: new Date(m.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
                  }))}>
                    <defs>
                      <linearGradient id="shippedGradient" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor="#8b5cf6" stopOpacity={0.3}/>
                        <stop offset="95%" stopColor="#8b5cf6" stopOpacity={0}/>
                      </linearGradient>
                    </defs>
                    <CartesianGrid strokeDasharray="3 3" stroke="#2A3D6B" />
                    <XAxis dataKey="timeLabel" stroke="#6B7280" fontSize={10} tickLine={false} axisLine={false} interval="preserveStartEnd" />
                    <YAxis stroke="#6B7280" fontSize={10} tickLine={false} axisLine={false} tickFormatter={(v) => v >= 1000 ? `${(v / 1000).toFixed(0)}k` : v} />
                    <Tooltip
                      contentStyle={{ backgroundColor: '#18284F', border: '1px solid #2A3D6B', borderRadius: '8px' }}
                      labelStyle={{ color: '#9CA3AF' }}
                      formatter={(value) => [value?.toLocaleString(), 'Shipped']}
                    />
                    <Area type="monotone" dataKey="results_shipped" stroke="#8b5cf6" fill="url(#shippedGradient)" strokeWidth={2} />
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            </div>
          </Card>
        )}

        {/* Executors */}
        {agent.executors && agent.executors.length > 0 && (
          <Card className="mb-6">
            <h3 className="text-sm font-medium text-theme-muted mb-4 flex items-center gap-2">
              <Cpu className="w-4 h-4" />
              Available Executors
            </h3>
            <div className="flex flex-wrap gap-2">
              {agent.executors.map((executor) => (
                <span
                  key={executor}
                  className="px-3 py-1.5 rounded-lg text-sm bg-surface-tertiary text-pilot-cyan border border-pilot-cyan/20"
                >
                  {executor}
                </span>
              ))}
            </div>
          </Card>
        )}

        {/* Capabilities */}
        <Card>
          <h3 className="text-sm font-medium text-theme-muted mb-4 flex items-center gap-2">
            <Shield className="w-4 h-4" />
            Agent Capabilities
          </h3>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div className="bg-surface-primary rounded-lg p-3 text-center">
              <Activity className="w-5 h-5 mx-auto mb-2 text-pilot-cyan" />
              <div className="text-sm text-theme-primary">ICMP Ping</div>
              <div className="text-xs text-theme-muted">Basic connectivity</div>
            </div>
            {agent.executors?.includes('mtr') && (
              <div className="bg-surface-primary rounded-lg p-3 text-center">
                <HardDrive className="w-5 h-5 mx-auto mb-2 text-status-healthy" />
                <div className="text-sm text-theme-primary">MTR</div>
                <div className="text-xs text-theme-muted">Path analysis</div>
              </div>
            )}
            {agent.executors?.includes('tcp') && (
              <div className="bg-surface-primary rounded-lg p-3 text-center">
                <Globe className="w-5 h-5 mx-auto mb-2 text-accent" />
                <div className="text-sm text-theme-primary">TCP Probe</div>
                <div className="text-xs text-theme-muted">Port connectivity</div>
              </div>
            )}
            {agent.executors?.includes('http') && (
              <div className="bg-surface-primary rounded-lg p-3 text-center">
                <Server className="w-5 h-5 mx-auto mb-2 text-pilot-red" />
                <div className="text-sm text-theme-primary">HTTP Check</div>
                <div className="text-xs text-theme-muted">Web requests</div>
              </div>
            )}
          </div>
        </Card>
      </PageContent>

      {/* Edit Agent Modal */}
      {showEditModal && (
        <EditAgentModal
          agent={agent}
          onClose={() => setShowEditModal(false)}
          onSave={async (updates) => {
            try {
              await endpoints.updateAgent(agent.id, updates);
              setShowEditModal(false);
              fetchAgent();
            } catch (err) {
              console.error('Failed to update agent:', err);
              throw err;
            }
          }}
        />
      )}
    </>
  );
}

function EditAgentModal({ agent, onClose, onSave }) {
  const [formData, setFormData] = useState({
    name: agent.name || '',
    region: agent.region || '',
    location: agent.location || '',
    provider: agent.provider || '',
    max_targets: agent.max_targets || 1000,
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      await onSave({
        ...formData,
        max_targets: parseInt(formData.max_targets, 10) || 1000,
      });
    } catch (err) {
      setError(err.message || 'Failed to save changes');
      setSaving(false);
    }
  };

  const handleChange = (field) => (e) => {
    setFormData((prev) => ({ ...prev, [field]: e.target.value }));
  };

  return (
    <Modal isOpen onClose={onClose} title="Edit Agent">
      <form onSubmit={handleSubmit} className="space-y-4">
        {error && (
          <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 text-sm text-red-400">
            {error}
          </div>
        )}

        <Input
          label="Name"
          value={formData.name}
          onChange={handleChange('name')}
          placeholder="Agent display name"
        />

        <Input
          label="Region"
          value={formData.region}
          onChange={handleChange('region')}
          placeholder="e.g., us-west-2, eu-central-1"
        />

        <Input
          label="Location"
          value={formData.location}
          onChange={handleChange('location')}
          placeholder="e.g., San Francisco, CA"
        />

        <Input
          label="Provider"
          value={formData.provider}
          onChange={handleChange('provider')}
          placeholder="e.g., AWS, GCP, On-Prem"
        />

        <Input
          label="Max Targets"
          type="number"
          value={formData.max_targets}
          onChange={handleChange('max_targets')}
          placeholder="Maximum targets this agent can monitor"
          min={1}
        />

        <div className="flex justify-end gap-3 pt-4">
          <Button type="button" variant="secondary" onClick={onClose} disabled={saving}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={saving}>
            {saving ? 'Saving...' : 'Save Changes'}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
